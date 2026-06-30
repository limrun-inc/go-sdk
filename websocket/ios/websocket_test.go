package ios

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSignalingURL(t *testing.T) {
	tests := []struct {
		name      string
		apiURL    string
		wantStart string
	}{
		{"https", "https://example.com/api", "wss://example.com/api/signaling"},
		{"http", "http://example.com", "ws://example.com/signaling"},
		{"trailing slash", "https://example.com/", "wss://example.com/signaling"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := signalingURL(tt.apiURL, "tok123")
			if err != nil {
				t.Fatalf("signalingURL: %v", err)
			}
			if !strings.HasPrefix(got, tt.wantStart) {
				t.Errorf("got %q, want prefix %q", got, tt.wantStart)
			}
			if !strings.Contains(got, "token=tok123") {
				t.Errorf("got %q, want token query", got)
			}
		})
	}
}

func TestNewSessionIDUnique(t *testing.T) {
	a, b := newSessionID(), newSessionID()
	if a == "" || b == "" {
		t.Fatal("session id is empty")
	}
	if a == b {
		t.Errorf("expected unique session ids, got %q twice", a)
	}
}

func TestDecodeBase64(t *testing.T) {
	if b, err := decodeBase64(""); err != nil || b != nil {
		t.Errorf("empty: got %v, %v; want nil, nil", b, err)
	}
	// "hi" base64 == "aGk="
	if b, err := decodeBase64("aGk="); err != nil || string(b) != "hi" {
		t.Errorf("got %q, %v; want \"hi\", nil", b, err)
	}
}

func TestScrollRequestMarshal(t *testing.T) {
	req := &request{
		Type:       "scroll",
		ID:         "abc",
		Direction:  ScrollDown,
		Pixels:     300,
		Coordinate: []float64{10, 20},
		Momentum:   0.5,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["direction"] != "down" {
		t.Errorf("direction = %v, want down", m["direction"])
	}
	if m["pixels"] != float64(300) {
		t.Errorf("pixels = %v, want 300", m["pixels"])
	}
	if m["momentum"] != 0.5 {
		t.Errorf("momentum = %v, want 0.5", m["momentum"])
	}
	// Unrelated optional fields must be omitted.
	if _, ok := m["bundleId"]; ok {
		t.Error("bundleId should be omitted")
	}
}

func TestActionConstructors(t *testing.T) {
	tap := ActionTap(5, 6)
	if tap.Type != "tap" || tap.X != 5 || tap.Y != 6 {
		t.Errorf("ActionTap = %+v", tap)
	}

	typeText := ActionTypeText("hello", true)
	if typeText.Type != "typeText" || typeText.Text != "hello" || !typeText.PressEnter {
		t.Errorf("ActionTypeText = %+v", typeText)
	}

	wait := ActionWait(500)
	if wait.Type != "wait" || wait.DurationMs != 500 {
		t.Errorf("ActionWait = %+v", wait)
	}

	scroll := ActionScroll(ScrollLeft, 120, &ScrollOptions{Coordinate: &[2]float64{1, 2}, Momentum: 0.25})
	if scroll.Type != "scroll" || scroll.Direction != ScrollLeft || scroll.Pixels != 120 {
		t.Errorf("ActionScroll = %+v", scroll)
	}
	if len(scroll.Coordinate) != 2 || scroll.Coordinate[0] != 1 || scroll.Coordinate[1] != 2 {
		t.Errorf("ActionScroll coordinate = %+v", scroll.Coordinate)
	}

	btn := ActionButtonDown("home")
	if btn.Type != "buttonDown" || btn.Button != "home" {
		t.Errorf("ActionButtonDown = %+v", btn)
	}
}

func TestPerformActionMarshalOmitsEmpty(t *testing.T) {
	data, err := json.Marshal(ActionTap(1, 2))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["type"] != "tap" {
		t.Errorf("type = %v, want tap", m["type"])
	}
	for _, k := range []string{"selector", "text", "key", "direction", "button"} {
		if _, ok := m[k]; ok {
			t.Errorf("field %q should be omitted, got %v", k, m[k])
		}
	}
}

func TestLaunchAppRuntimeRequiresRelaunch(t *testing.T) {
	c := &Client{}
	err := c.LaunchApp(context.Background(), "com.example.app",
		WithLaunchRuntime(LaunchAppRuntime{Kind: "detox", ServerURL: "ws://x", SessionID: "s"}),
		WithLaunchMode(LaunchModeForegroundIfRunning),
	)
	if err == nil {
		t.Fatal("expected error for runtime launch with ForegroundIfRunning")
	}
	if !strings.Contains(err.Error(), "RelaunchIfRunning") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartRecordingQualityValidation(t *testing.T) {
	c := &Client{}
	for _, q := range []int{1, 4, 11, 100} {
		if err := c.StartRecording(context.Background(), &RecordingOptions{Quality: q}); err == nil {
			t.Errorf("quality %d: expected validation error", q)
		}
	}
}

func TestKeepAliveNotConnected(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)
	if err := c.KeepAlive(); err != ErrNotConnected {
		t.Errorf("KeepAlive = %v, want ErrNotConnected", err)
	}
}

func TestFirstActionError(t *testing.T) {
	if err := firstActionError(nil); err != nil {
		t.Errorf("nil results: got %v, want nil", err)
	}
	ok := []PerformActionResult{{Type: "tap"}, {Type: "typeText"}}
	if err := firstActionError(ok); err != nil {
		t.Errorf("all-ok results: got %v, want nil", err)
	}
	failing := []PerformActionResult{
		{Type: "tap"},
		{Type: "tapElement", Error: "element not found"},
		{Type: "typeText", Error: "ignored second error"},
	}
	err := firstActionError(failing)
	if err == nil {
		t.Fatal("expected error for failing batch")
	}
	if !strings.Contains(err.Error(), "tapElement") || !strings.Contains(err.Error(), "element not found") {
		t.Errorf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "ignored second error") {
		t.Errorf("should report only the first failure: %v", err)
	}
}

// TestLogStreamTerminatesOnServerError verifies that a server-side error
// message terminates the stream: Err receives the error and Lines closes even
// though the server keeps the underlying socket open.
func TestLogStreamTerminatesOnServerError(t *testing.T) {
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		_ = conn.WriteJSON(map[string]any{"type": "streamSyslog", "error": "boom"})
		// Keep the socket open so termination is driven by the error message,
		// not by a connection close.
		time.Sleep(2 * time.Second)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{apiURL: srv.URL, token: "t", httpClient: http.DefaultClient}
	stream, err := c.StreamSyslog(context.Background())
	if err != nil {
		t.Fatalf("StreamSyslog: %v", err)
	}

	select {
	case e := <-stream.Err():
		if e == nil || !strings.Contains(e.Error(), "boom") {
			t.Errorf("Err = %v, want error containing boom", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream error")
	}

	select {
	case _, ok := <-stream.Lines():
		if ok {
			t.Error("expected Lines channel to be closed after terminal error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Lines channel not closed after terminal error")
	}
}
