package android

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// mockInstance is a minimal Android control endpoint. handle is called for
// every message the client sends and may write replies on conn.
type mockInstance struct {
	t      *testing.T
	srv    *httptest.Server
	handle func(conn *websocket.Conn, msg map[string]any)

	mu       sync.Mutex
	received []map[string]any
	conn     *websocket.Conn
}

func newMockInstance(t *testing.T, handle func(conn *websocket.Conn, msg map[string]any)) *mockInstance {
	t.Helper()
	m := &mockInstance{t: t, handle: handle}
	upgrader := websocket.Upgrader{}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" || r.URL.Query().Get("token") != "tok" {
			http.Error(w, "bad path or token", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		m.mu.Lock()
		m.conn = conn
		m.mu.Unlock()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg map[string]any
			if err := json.Unmarshal(data, &msg); err != nil {
				t.Errorf("client sent invalid JSON: %s", data)
				continue
			}
			m.mu.Lock()
			m.received = append(m.received, msg)
			m.mu.Unlock()
			if m.handle != nil {
				m.handle(conn, msg)
			}
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mockInstance) client(t *testing.T, opts ...Option) *Client {
	t.Helper()
	c, err := NewClient(m.srv.URL, "tok", opts...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func (m *mockInstance) messages() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]map[string]any(nil), m.received...)
}

func reply(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	if err := conn.WriteJSON(v); err != nil {
		t.Errorf("reply: %v", err)
	}
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestControlURL(t *testing.T) {
	tests := []struct {
		name, apiURL, want string
	}{
		{"https", "https://inst.example.com", "wss://inst.example.com/ws?token=t%2Bk"},
		{"http", "http://inst.example.com", "ws://inst.example.com/ws?token=t%2Bk"},
		{"trailing slash", "https://inst.example.com/", "wss://inst.example.com/ws?token=t%2Bk"},
		{"base path", "https://gw.example.com/instances/abc", "wss://gw.example.com/instances/abc/ws?token=t%2Bk"},
		{"existing query dropped", "https://inst.example.com/?x=1#frag", "wss://inst.example.com/ws?token=t%2Bk"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := controlURL(tt.apiURL, "t+k")
			if err != nil {
				t.Fatalf("controlURL: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	if _, err := controlURL("ftp://x", "t"); err == nil {
		t.Error("expected an error for a non-http scheme")
	}
}

func TestDialRejectsBadToken(t *testing.T) {
	m := newMockInstance(t, nil)
	if _, err := NewClient(m.srv.URL, "wrong"); err == nil {
		t.Fatal("expected dial to fail with a bad token")
	}
}

func TestInstallApp(t *testing.T) {
	m := newMockInstance(t, func(conn *websocket.Conn, msg map[string]any) {
		if msg["type"] != "asset" {
			return
		}
		url := msg["url"].(string)
		if strings.Contains(url, "broken") {
			reply(t, conn, map[string]any{"type": "assetResult", "result": "failure", "url": url, "message": "INSTALL_FAILED_INVALID_APK"})
			return
		}
		reply(t, conn, map[string]any{"type": "assetResult", "result": "success", "url": url})
	})
	c := m.client(t)

	if err := c.InstallApp(ctx(t), "https://cdn.example.com/app.apk"); err != nil {
		t.Fatalf("InstallApp: %v", err)
	}
	err := c.InstallApp(ctx(t), "https://cdn.example.com/broken.apk")
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Message != "INSTALL_FAILED_INVALID_APK" {
		t.Fatalf("got %v, want CommandError with the server message", err)
	}

	sent := m.messages()[0]
	if len(sent) != 2 || sent["type"] != "asset" || sent["url"] != "https://cdn.example.com/app.apk" {
		t.Errorf("asset request = %v, want exactly {type: asset, url}", sent)
	}
}

// Installs are acknowledged by URL with no request id, so two installs of the
// same URL must resolve in the order they were sent.
func TestInstallAppSameURLResolvesInOrder(t *testing.T) {
	gotFirst := make(chan struct{})
	release := make(chan struct{})
	m := newMockInstance(t, func(conn *websocket.Conn, msg map[string]any) {
		if msg["type"] != "asset" {
			return
		}
		select {
		case <-gotFirst:
			// second request: answer both, first success then failure
			<-release
			reply(t, conn, map[string]any{"type": "assetResult", "result": "success", "url": msg["url"]})
			reply(t, conn, map[string]any{"type": "assetResult", "result": "failure", "url": msg["url"], "message": "second"})
		default:
			close(gotFirst)
		}
	})
	c := m.client(t)
	const u = "https://cdn.example.com/same.apk"

	first := make(chan error, 1)
	go func() { first <- c.InstallApp(ctx(t), u) }()
	<-gotFirst
	second := make(chan error, 1)
	go func() { second <- c.InstallApp(ctx(t), u) }()
	close(release)

	if err := <-first; err != nil {
		t.Errorf("first install: got %v, want success", err)
	}
	if err := <-second; err == nil || !strings.Contains(err.Error(), "second") {
		t.Errorf("second install: got %v, want the failure", err)
	}
}

// A timed-out install is still on the wire; its late assetResult must not
// settle the next install of the same URL.
func TestInstallAppLateResultAfterTimeout(t *testing.T) {
	var n atomic.Int32
	release := make(chan struct{})
	m := newMockInstance(t, func(conn *websocket.Conn, msg map[string]any) {
		if msg["type"] != "asset" {
			return
		}
		if n.Add(1) == 1 {
			return // hold the first answer until the second install is sent
		}
		<-release
		reply(t, conn, map[string]any{"type": "assetResult", "result": "failure", "url": msg["url"], "message": "late result of the first install"})
		reply(t, conn, map[string]any{"type": "assetResult", "result": "success", "url": msg["url"]})
	})
	c := m.client(t)
	const u = "https://cdn.example.com/slow.apk"

	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.InstallApp(short, u); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first install: got %v, want deadline exceeded", err)
	}
	second := make(chan error, 1)
	go func() { second <- c.InstallApp(ctx(t), u) }()
	waitFor(t, func() bool { return n.Load() == 2 })
	close(release)
	if err := <-second; err != nil {
		t.Fatalf("second install got the first install's late result: %v", err)
	}
}

// Many concurrent installs of one URL: the i-th result must reach the i-th
// request on the wire, whatever the goroutine scheduling.
func TestInstallAppConcurrentSameURLMatchesWireOrder(t *testing.T) {
	const total = 50
	var sent atomic.Int32
	m := newMockInstance(t, func(conn *websocket.Conn, msg map[string]any) {
		if msg["type"] != "asset" {
			return
		}
		// answer each request with its own position on the wire
		i := sent.Add(1)
		reply(t, conn, map[string]any{"type": "assetResult", "result": "failure", "url": msg["url"], "message": fmt.Sprint(i)})
	})
	c := m.client(t)

	var wg sync.WaitGroup
	got := make(chan string, total)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var ce *CommandError
			if err := c.InstallApp(ctx(t), "https://cdn.example.com/same.apk"); errors.As(err, &ce) {
				got <- ce.Message
			}
		}()
	}
	wg.Wait()
	close(got)
	seen := map[string]bool{}
	for msg := range got {
		if seen[msg] {
			t.Fatalf("result %s delivered twice", msg)
		}
		seen[msg] = true
	}
	if len(seen) != total {
		t.Fatalf("got %d distinct results, want %d", len(seen), total)
	}
}

func TestOpenURL(t *testing.T) {
	m := newMockInstance(t, func(conn *websocket.Conn, msg map[string]any) {
		if msg["type"] == "openUrl" {
			reply(t, conn, map[string]any{"type": "openUrlResult", "id": msg["id"], "payload": map[string]any{"url": msg["url"]}})
		}
	})
	c := m.client(t)

	got, err := c.OpenURL(ctx(t), "myapp://settings")
	if err != nil {
		t.Fatalf("OpenURL: %v", err)
	}
	if got != "myapp://settings" {
		t.Errorf("OpenURL = %q", got)
	}

	// params go out flat and under payload, like the TypeScript SDK
	sent := m.messages()[0]
	payload, _ := sent["payload"].(map[string]any)
	if sent["url"] != "myapp://settings" || payload["url"] != "myapp://settings" {
		t.Errorf("request = %v, want url both flat and in payload", sent)
	}
	if id, _ := sent["id"].(string); !strings.HasPrefix(id, "go-client-") {
		t.Errorf("request id = %v", sent["id"])
	}
}

func TestCommandErrors(t *testing.T) {
	m := newMockInstance(t, func(conn *websocket.Conn, msg map[string]any) {
		switch msg["type"] {
		case "openUrl":
			reply(t, conn, map[string]any{"type": "openUrlResult", "id": msg["id"],
				"error": map[string]any{"code": "ACTIVITY_NOT_FOUND", "message": "no activity handles myapp://", "retriable": false}})
		case "terminateApp":
			reply(t, conn, map[string]any{"type": "terminateAppResult", "id": msg["id"],
				"error": map[string]any{"code": "BUSY", "retriable": true}})
		case "launchApp":
			// flat result: the error is a plain string
			reply(t, conn, map[string]any{"type": "launchAppResult", "id": msg["id"], "error": "package not installed"})
		case "screenshot":
			reply(t, conn, map[string]any{"type": "screenshotError", "id": msg["id"], "message": "display off"})
		}
	})
	c := m.client(t)
	var ce *CommandError

	_, err := c.OpenURL(ctx(t), "myapp://x")
	if !errors.As(err, &ce) || ce.Code != "ACTIVITY_NOT_FOUND" || ce.Message != "no activity handles myapp://" || ce.Retriable {
		t.Errorf("object error: got %#v", err)
	}
	err = c.TerminateApp(ctx(t), "com.example")
	if !errors.As(err, &ce) || ce.Code != "BUSY" || !ce.Retriable {
		t.Errorf("code-only error: got %#v", err)
	}
	err = c.LaunchApp(ctx(t), "com.example", "")
	if !errors.As(err, &ce) || ce.Message != "package not installed" {
		t.Errorf("string error: got %#v", err)
	}
	_, err = c.Screenshot(ctx(t))
	if !errors.As(err, &ce) || ce.Message != "display off" {
		t.Errorf("screenshotError: got %#v", err)
	}
}

func TestLaunchListTerminate(t *testing.T) {
	m := newMockInstance(t, func(conn *websocket.Conn, msg map[string]any) {
		switch msg["type"] {
		case "launchApp":
			reply(t, conn, map[string]any{"type": "launchAppResult", "id": msg["id"], "payload": map[string]any{"packageName": msg["packageName"]}})
		case "terminateApp":
			reply(t, conn, map[string]any{"type": "terminateAppResult", "id": msg["id"], "payload": map[string]any{}})
		case "listApps":
			reply(t, conn, map[string]any{"type": "listAppsResult", "id": msg["id"], "payload": map[string]any{
				"apps": []any{map[string]any{"bundleId": "com.example", "name": "Example", "installType": "user", "installedAt": 1790000000000}},
			}})
		}
	})
	c := m.client(t)

	if err := c.LaunchApp(ctx(t), "com.example", LaunchModeRelaunchIfRunning); err != nil {
		t.Fatalf("LaunchApp: %v", err)
	}
	apps, err := c.ListApps(ctx(t))
	if err != nil || len(apps) != 1 || apps[0].BundleID != "com.example" || apps[0].InstalledAt != 1790000000000 {
		t.Fatalf("ListApps = %v, %v", apps, err)
	}
	if err := c.TerminateApp(ctx(t), "com.example"); err != nil {
		t.Fatalf("TerminateApp: %v", err)
	}

	launch := m.messages()[0]
	if launch["mode"] != "RelaunchIfRunning" || launch["packageName"] != "com.example" {
		t.Errorf("launch request = %v", launch)
	}
	list := m.messages()[1]
	if _, hasPayload := list["payload"]; hasPayload {
		t.Errorf("parameterless request should carry no payload: %v", list)
	}
}

func TestScreenshot(t *testing.T) {
	var legacy atomic.Bool
	m := newMockInstance(t, func(conn *websocket.Conn, msg map[string]any) {
		if msg["type"] != "screenshot" {
			return
		}
		if legacy.Load() {
			reply(t, conn, map[string]any{"type": "screenshot", "id": msg["id"], "dataUri": "data:image/png;base64,b2xk"})
			return
		}
		reply(t, conn, map[string]any{"type": "screenshotResult", "id": msg["id"], "payload": map[string]any{"dataUri": "data:image/png;base64,bmV3"}})
	})
	c := m.client(t)

	s, err := c.Screenshot(ctx(t))
	if err != nil || s.DataURI != "data:image/png;base64,bmV3" {
		t.Fatalf("Screenshot = %v, %v", s, err)
	}
	legacy.Store(true)
	s, err = c.Screenshot(ctx(t))
	if err != nil || s.DataURI != "data:image/png;base64,b2xk" {
		t.Fatalf("legacy Screenshot = %v, %v", s, err)
	}
}

func TestKeepAlive(t *testing.T) {
	m := newMockInstance(t, nil)
	c := m.client(t)
	if err := c.KeepAlive(); err != nil {
		t.Fatalf("KeepAlive: %v", err)
	}
	waitFor(t, func() bool { return len(m.messages()) == 1 })
	msg := m.messages()[0]
	if msg["type"] != "keepAlive" || msg["sessionId"] == "" || msg["sessionId"] == nil {
		t.Errorf("keepAlive message = %v", msg)
	}
}

func TestKeepAliveInterval(t *testing.T) {
	m := newMockInstance(t, nil)
	c := m.client(t, WithKeepAliveInterval(20*time.Millisecond))
	waitFor(t, func() bool { return len(m.messages()) >= 3 })

	session := m.messages()[0]["sessionId"]
	for _, msg := range m.messages() {
		if msg["type"] != "keepAlive" || msg["sessionId"] != session {
			t.Fatalf("expected keepAlive messages with one session id, got %v", msg)
		}
	}
	_ = c.Close()
	n := len(m.messages())
	time.Sleep(80 * time.Millisecond)
	if len(m.messages()) != n {
		t.Error("keepAlive kept firing after Close")
	}
}

func TestCloseFailsPendingRequests(t *testing.T) {
	received := make(chan struct{}, 2)
	m := newMockInstance(t, func(*websocket.Conn, map[string]any) { received <- struct{}{} }) // never answers
	c := m.client(t)

	cmdErr := make(chan error, 1)
	go func() { _, err := c.OpenURL(context.Background(), "https://example.com"); cmdErr <- err }()
	assetErr := make(chan error, 1)
	go func() { assetErr <- c.InstallApp(context.Background(), "https://cdn.example.com/a.apk") }()
	<-received
	<-received

	_ = c.Close()
	if err := <-cmdErr; !errors.Is(err, ErrConnectionClose) {
		t.Errorf("command: got %v, want ErrConnectionClose", err)
	}
	if err := <-assetErr; !errors.Is(err, ErrConnectionClose) {
		t.Errorf("install: got %v, want ErrConnectionClose", err)
	}
	if _, err := c.OpenURL(ctx(t), "https://example.com"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("after close: got %v, want ErrNotConnected", err)
	}
}

func TestServerDisconnectFailsPendingRequests(t *testing.T) {
	m := newMockInstance(t, func(conn *websocket.Conn, msg map[string]any) {
		_ = conn.Close() // drop the connection instead of answering
	})
	c := m.client(t)
	if _, err := c.OpenURL(ctx(t), "https://example.com"); !errors.Is(err, ErrConnectionClose) {
		t.Errorf("got %v, want ErrConnectionClose", err)
	}
	// after the drop the client is closed: new calls fail fast
	start := time.Now()
	if _, err := c.OpenURL(ctx(t), "https://example.com"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("after disconnect: got %v, want ErrNotConnected", err)
	}
	if err := c.InstallApp(ctx(t), "https://cdn.example.com/a.apk"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("install after disconnect: got %v, want ErrNotConnected", err)
	}
	if time.Since(start) > time.Second {
		t.Error("calls after disconnect waited instead of failing fast")
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close after disconnect: %v", err)
	}
}

func TestResultForUnknownIDIsIgnored(t *testing.T) {
	m := newMockInstance(t, func(conn *websocket.Conn, msg map[string]any) {
		if msg["type"] == "openUrl" {
			reply(t, conn, map[string]any{"type": "openUrlResult", "id": "someone-else", "payload": map[string]any{"url": "wrong"}})
			reply(t, conn, map[string]any{"type": "openUrlResult", "id": msg["id"], "payload": map[string]any{"url": "right"}})
		}
	})
	c := m.client(t)
	got, err := c.OpenURL(ctx(t), "right")
	if err != nil || got != "right" {
		t.Fatalf("OpenURL = %q, %v", got, err)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
