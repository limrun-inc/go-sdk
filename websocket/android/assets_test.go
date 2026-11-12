package android

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func assetClient(t *testing.T, handle func(*websocket.Conn, map[string]string)) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			t.Errorf("unexpected HTTP call: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			var req map[string]string
			if ws.ReadJSON(&req) != nil {
				return
			}
			handle(ws, req)
		}
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, "test", WithReconnect(0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestSendAssetSources(t *testing.T) {
	frames := make(chan map[string]string, 3)
	client := assetClient(t, func(ws *websocket.Conn, req map[string]string) {
		frames <- req
		reply := map[string]string{"type": "assetResult", "id": req["id"], "result": "success"}
		if req["assetName"] == "denied.apk" {
			reply["result"] = "failure"
			reply["message"] = "asset read denied"
		}
		_ = ws.WriteJSON(reply)
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.InstallAppByName(ctx, "team/app:prod+版本.apk"); err != nil {
		t.Fatal(err)
	}
	if err := client.SendAsset(ctx, "https://storage.example/app.apk"); err != nil {
		t.Fatal(err)
	}
	if err := client.SendAssetByName(ctx, "denied.apk"); err == nil || !strings.Contains(err.Error(), "read denied") {
		t.Fatalf("denial: %v", err)
	}
	named, url, denied := <-frames, <-frames, <-frames
	if named["assetName"] != "team/app:prod+版本.apk" || named["url"] != "" || url["url"] != "https://storage.example/app.apk" || url["assetName"] != "" {
		t.Fatalf("sources: %v %v", named, url)
	}
	if named["id"] == "" || url["id"] == "" || denied["id"] == "" || named["id"] == url["id"] {
		t.Fatalf("IDs: %v %v %v", named, url, denied)
	}
	if err := new(Client).SendAssetByName(ctx, ""); err == nil {
		t.Fatal("accepted empty asset name")
	}
	if err := new(Client).InstallAppByName(ctx, ""); err == nil {
		t.Fatal("accepted empty asset name")
	}
}

func TestAssetCorrelation(t *testing.T) {
	for _, mode := range []string{"name", "url", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			firstReceived := make(chan struct{})
			var first map[string]string
			client := assetClient(t, func(ws *websocket.Conn, req map[string]string) {
				if first == nil {
					first = req
					close(firstReceived)
					return
				}
				failure := map[string]string{"type": "assetResult", "result": "failure", "message": "first failed", "url": first["url"]}
				success := map[string]string{"type": "assetResult", "result": "success", "url": req["url"]}
				if mode == "legacy" {
					_ = ws.WriteJSON(failure)
					_ = ws.WriteJSON(success)
					return
				}
				// An unrecognized ID must not settle a pending request with the same URL.
				_ = ws.WriteJSON(map[string]string{"type": "assetResult", "id": "unknown", "url": req["url"], "result": "success"})
				success["id"] = req["id"]
				failure["id"] = first["id"]
				_ = ws.WriteJSON(success)
				_ = ws.WriteJSON(failure)
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			install := func() error {
				if mode == "name" {
					return client.InstallAppByName(ctx, "same.apk")
				}
				return client.SendAsset(ctx, "https://storage.example/same.apk")
			}
			firstDone := make(chan error, 1)
			go func() { firstDone <- install() }()
			select {
			case <-firstReceived:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if err := install(); err != nil {
				t.Fatalf("second: %v", err)
			}
			if err := <-firstDone; err == nil || err.Error() != "first failed" {
				t.Fatalf("first: %v", err)
			}
		})
	}
}

func TestAssetLateReplyCannotCompleteRetry(t *testing.T) {
	var first map[string]string
	client := assetClient(t, func(ws *websocket.Conn, req map[string]string) {
		if first == nil {
			first = req
			return
		}
		_ = ws.WriteJSON(map[string]string{"type": "assetResult", "id": first["id"], "url": req["url"], "result": "success"})
		_ = ws.WriteJSON(map[string]string{"type": "assetResult", "id": req["id"], "url": req["url"], "result": "failure", "message": "retry failed"})
	})
	if err := client.SendAsset(context.Background(), "https://storage.example/same.apk", 30*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", err)
	}
	if err := client.SendAsset(context.Background(), "https://storage.example/same.apk", time.Second); err == nil || err.Error() != "retry failed" {
		t.Fatalf("retry: %v", err)
	}
}

func TestAssetDisconnectClearsPending(t *testing.T) {
	client := assetClient(t, func(ws *websocket.Conn, _ map[string]string) { ws.Close() })
	err := client.InstallAppByName(context.Background(), "app.apk", time.Second)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("disconnect: %v", err)
	}
	client.assetsMu.Lock()
	defer client.assetsMu.Unlock()
	if len(client.assets) != 0 {
		t.Fatalf("pending after disconnect: %d", len(client.assets))
	}
}

func TestMalformedAssetIDDoesNotFallBackToURL(t *testing.T) {
	for _, id := range []string{`null`, `123`, `""`} {
		pending := &assetPending{id: "valid", url: "https://storage.example/app.apk", done: make(chan error, 1)}
		client := &Client{assets: []*assetPending{pending}}
		client.handleMessage(json.RawMessage(`{"type":"assetResult","id":` + id + `,"url":"https://storage.example/app.apk","result":"success"}`))
		select {
		case <-pending.done:
			t.Fatalf("accepted malformed id %s", id)
		default:
		}
	}
}
