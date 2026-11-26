package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// refusing serves a signaling socket that answers the first message as an
// instance refuses a command above the token's grant.
func refusing(t *testing.T, frame bool) (string, *atomic.Int32) {
	t.Helper()
	var conns atomic.Int32
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conns.Add(1)
		defer ws.Close()
		if _, _, err := ws.ReadMessage(); err != nil {
			return
		}
		if frame {
			_ = ws.WriteJSON(map[string]string{"type": "error", "message": "this token grants watch only; the session is closed"})
		}
		_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, ""))
		time.Sleep(50 * time.Millisecond)
	}))
	t.Cleanup(srv.Close)
	endpoint, err := URL(srv.URL, "t", "signaling")
	if err != nil {
		t.Fatal(err)
	}
	return endpoint, &conns
}

func TestPolicyCloseDoesNotReconnect(t *testing.T) {
	for name, tc := range map[string]struct {
		frame bool
		want  string
	}{
		"error frame":     {true, "this token grants watch only; the session is closed"},
		"bare 1008 close": {false, "the instance closed the session"},
	} {
		t.Run(name, func(t *testing.T) {
			endpoint, conns := refusing(t, tc.frame)
			permanent := make(chan struct{}, 1)
			c, err := New(context.Background(), endpoint, Options{
				MaxReconnectAttempts:  3,
				ReconnectDelay:        10 * time.Millisecond,
				OnPermanentDisconnect: func() { permanent <- struct{}{} },
			})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, err = c.Call(context.Background(), "tap", nil, time.Second)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			select {
			case <-permanent:
			case <-time.After(time.Second):
				t.Fatal("the client did not give up")
			}
			time.Sleep(100 * time.Millisecond)
			if n := conns.Load(); n != 1 {
				t.Errorf("connections = %d, want 1", n)
			}
			if c.State() != Disconnected {
				t.Errorf("state = %s", c.State())
			}
		})
	}
}
