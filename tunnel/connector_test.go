package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestConnectorResumesWithoutDroppingInstance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	upgrader := websocket.Upgrader{}
	var instanceDials atomic.Int32
	instance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := upgrader.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		instanceDials.Add(1)
		_, raw, e := ws.ReadMessage()
		if e != nil {
			return
		}
		m, e := DecodeClientMessage(raw)
		if e != nil {
			t.Error(e)
			return
		}
		if m.Name != "private-test" {
			t.Errorf("tunnel name %q", m.Name)
		}
		s, _ := ParseSelectors(m.SelectorValues, ValidateOptions{})
		hash, _ := ConfigHashWithInspection(s, m.Inspection)
		ready, _ := EncodeServerMessage(ServerMessage{Type: "ready", Version: Version, TunnelID: "destination-1", ConfigHash: hash, Selectors: SelectorReports(s)})
		ws.WriteMessage(websocket.TextMessage, ready)
		for {
			if _, _, e = ws.ReadMessage(); e != nil {
				return
			}
		}
	}))
	defer instance.Close()
	var controlDials atomic.Int32
	resumed := make(chan error, 1)
	bye := make(chan struct{}, 1)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/organizations/org/tunnels/private-id/connect" {
			t.Error(r.URL.Path)
		}
		ws, e := upgrader.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		ws.SetReadDeadline(time.Now().Add(7 * time.Second))
		dial := controlDials.Add(1)
		var hello map[string]any
		if e = ws.ReadJSON(&hello); e != nil {
			return
		}
		if hello["version"] != float64(2) {
			t.Error(hello)
		}
		if dial == 2 {
			attached, _ := hello["attached"].([]any)
			if hello["sessionId"] != "session-1" || hello["replace"] != false || len(attached) != 1 || attached[0] != "instance-1" {
				resumed <- fmt.Errorf("resume hello: %v", hello)
				return
			}
		}
		ws.WriteJSON(map[string]any{"type": "active", "sessionId": "session-1", "leaseSeconds": 60, "name": "private-test"})
		ws.WriteJSON(map[string]any{"type": "attach", "instanceId": "instance-1", "url": instance.URL, "selectors": []string{"localhost:3000"}, "token": "instance-token", "expiresInSeconds": 60})
		var attached map[string]any
		if e = ws.ReadJSON(&attached); e != nil {
			resumed <- e
			return
		}
		if attached["type"] != "attached" || attached["tunnelId"] != "destination-1" {
			resumed <- fmt.Errorf("attach report: %v", attached)
			return
		}
		if dial == 1 {
			ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(1012, "restart"), time.Now().Add(time.Second))
			return
		}
		resumed <- nil
		for {
			_, raw, e := ws.ReadMessage()
			if e != nil {
				return
			}
			var m map[string]any
			json.Unmarshal(raw, &m)
			if m["type"] == "bye" {
				bye <- struct{}{}
				return
			}
		}
	}))
	defer hub.Close()
	connector, err := RunTunnel(ctx, ConnectorOptions{BaseURL: hub.URL, Token: "hub-token", OrganizationID: "org", TunnelID: "private-id", Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	defer connector.Close()
	select {
	case err = <-resumed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if instanceDials.Load() != 1 {
		t.Fatalf("instance dialed %d times", instanceDials.Load())
	}
	if err = connector.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-bye:
	case <-ctx.Done():
		t.Fatal("bye not sent")
	}
}
func TestConnectorRejectsTerminalUpgrade(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "token expired", http.StatusUnauthorized) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connector, e := RunTunnel(ctx, ConnectorOptions{BaseURL: server.URL, OrganizationID: "o", TunnelID: "t", Token: "expired"})
	if e != nil {
		t.Fatal(e)
	}
	if e = connector.Wait(ctx); e == nil || !strings.Contains(e.Error(), "refused") {
		t.Fatal(e)
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
