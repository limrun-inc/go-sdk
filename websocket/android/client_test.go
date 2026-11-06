package android

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestAndroidFramesAndCrashEvents(t *testing.T) {
	frames := make(chan map[string]json.RawMessage, 8)
	exits := make(chan AppExitInfo, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" || r.URL.Query().Get("token") != "test" {
			t.Errorf("URL=%s", r.URL)
		}
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			var req map[string]json.RawMessage
			if ws.ReadJSON(&req) != nil {
				return
			}
			frames <- req
			var id, kind, exec string
			json.Unmarshal(req["id"], &id)
			json.Unmarshal(req["type"], &kind)
			json.Unmarshal(req["execId"], &exec)
			if kind == "launchApp" {
				_ = ws.WriteJSON(map[string]any{"type": "appExit", "execId": exec, "packageName": "com.test", "reason": "anr", "logs": []string{"ANR"}, "anr": map[string]any{"pid": 42, "processStats": "blocked"}})
			}
			_ = ws.WriteJSON(map[string]any{"type": kind + "Result", "id": id, "payload": map[string]any{}})
		}
	}))
	defer srv.Close()
	c, err := NewClient(srv.URL, "test", WithReconnect(0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err = c.Tap(ctx, At(0, 0)); err != nil {
		t.Fatal(err)
	}
	frame := <-frames
	if string(frame["x"]) != "0" || string(frame["y"]) != "0" {
		t.Fatalf("zero coordinates lost: %v", frame)
	}
	var payload map[string]any
	if json.Unmarshal(frame["payload"], &payload) != nil || payload["x"] != float64(0) {
		t.Fatalf("missing duplicated payload: %s", frame["payload"])
	}
	if _, err = c.LaunchApp(ctx, "com.test", &LaunchAppOptions{OnExit: func(_ []string, info AppExitInfo) { exits <- info }}); err != nil {
		t.Fatal(err)
	}
	select {
	case info := <-exits:
		if info.ANR == nil || info.ANR.PID != 42 {
			t.Fatalf("ANR=%+v", info)
		}
	case <-ctx.Done():
		t.Fatal("exit not delivered")
	}
}
