package ios

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func fixtureClient(t *testing.T, handle func(*websocket.Conn, map[string]json.RawMessage)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var req map[string]json.RawMessage
			if conn.ReadJSON(&req) != nil {
				return
			}
			handle(conn, req)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, "test", WithReconnect(0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func field(m map[string]json.RawMessage, k string) string {
	var s string
	_ = json.Unmarshal(m[k], &s)
	return s
}
func TestZeroCoordinatesAndEmptyTextReachWire(t *testing.T) {
	requests := make(chan map[string]json.RawMessage, 4)
	c := fixtureClient(t, func(ws *websocket.Conn, req map[string]json.RawMessage) {
		requests <- req
		_ = ws.WriteJSON(map[string]any{"id": field(req, "id"), "type": "ok"})
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Tap(ctx, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetElementValue(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PerformActions(ctx, []PerformAction{ActionTap(0, 0), ActionTypeText("", false)}); err != nil {
		t.Fatal(err)
	}
	tap := <-requests
	if string(tap["x"]) != "0" || string(tap["y"]) != "0" {
		t.Fatalf("zero coordinates lost: %v", tap)
	}
	text := <-requests
	if string(text["text"]) != `""` || string(text["focused"]) != "true" {
		t.Fatalf("focused empty text lost: %v", text)
	}
	batch := <-requests
	var actions []map[string]json.RawMessage
	_ = json.Unmarshal(batch["actions"], &actions)
	if string(actions[0]["x"]) != "0" || string(actions[1]["text"]) != `""` {
		t.Fatalf("batch fields lost: %s", batch["actions"])
	}
}
func TestMonitorRoutesCrashAndHangByExecIDAcrossExits(t *testing.T) {
	exits := make(chan AppExitInfo, 2)
	hangs := make(chan AppHangInfo, 1)
	c := fixtureClient(t, func(ws *websocket.Conn, req map[string]json.RawMessage) {
		id := field(req, "id")
		exec := field(req, "execId")
		_ = ws.WriteJSON(map[string]any{"id": id, "type": "ok"})
		if field(req, "type") == "monitorApp" {
			_ = ws.WriteJSON(map[string]any{"type": "appHang", "execId": exec, "bundleId": "com.test", "hang": map[string]any{"pid": 7, "hangId": 1, "durationMs": 1000, "ended": false, "mainThreadCpuMs": 0}})
			for i := 0; i < 2; i++ {
				_ = ws.WriteJSON(map[string]any{"type": "appExit", "execId": exec, "bundleId": "com.test", "reason": "crash", "logs": []string{"fatal"}, "crash": map[string]any{"pid": 7, "stackTrace": "frame"}})
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	watch, err := c.MonitorApp(ctx, "com.test", AppMonitorCallbacks{OnExit: func(logs []string, info AppExitInfo) {
		if len(logs) != 1 {
			t.Errorf("logs=%v", logs)
		}
		exits <- info
	}, OnHang: func(info AppHangInfo) { hangs <- info }})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case info := <-exits:
			if info.Crash == nil || info.Crash.StackTrace != "frame" {
				t.Errorf("crash=%+v", info)
			}
		case <-ctx.Done():
			t.Fatal("missing repeated monitor exit")
		}
	}
	select {
	case info := <-hangs:
		if info.BundleID != "com.test" || info.MainThreadCPUMs == nil || *info.MainThreadCPUMs != 0 {
			t.Errorf("hang=%+v", info)
		}
	case <-ctx.Done():
		t.Fatal("missing hang")
	}
	if err = watch.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}
func TestDisconnectFailsPendingAndSimctlCancellationCompletes(t *testing.T) {
	c := fixtureClient(t, func(ws *websocket.Conn, req map[string]json.RawMessage) {
		if field(req, "type") == "screenshot" {
			ws.Close()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := c.Screenshot(ctx)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pending request was not failed on disconnect: %v", err)
	}
	c2 := fixtureClient(t, func(ws *websocket.Conn, req map[string]json.RawMessage) {})
	ctx2, cancel2 := context.WithCancel(context.Background())
	cmd := c2.Simctl(ctx2, "spawn", "booted", "log", "stream")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	cancel2()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Wait=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("simctl wait did not finish after cancellation")
	}
}
func TestSimctlPipeDoesNotBlockCommandReplies(t *testing.T) {
	c := fixtureClient(t, func(ws *websocket.Conn, req map[string]json.RawMessage) {
		if field(req, "type") == "simctl" {
			code := 0
			_ = ws.WriteJSON(map[string]any{"type": "simctlStream", "id": field(req, "id"), "stdout": "aGk=", "exitCode": code})
		} else {
			_ = ws.WriteJSON(map[string]any{"id": field(req, "id"), "base64": "image"})
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := c.Simctl(ctx, "listapps", "booted")
	pipe, _ := cmd.StdoutPipe()
	defer pipe.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Screenshot(ctx); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(pipe)
	if err != nil || string(data) != "hi" {
		t.Fatalf("output=%q err=%v", data, err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
