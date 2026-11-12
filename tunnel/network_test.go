package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func wsURL(s string) string { return "ws" + strings.TrimPrefix(s, "http") }

func TestRejectedWebSocketUpgradeRetainsStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		for _, body := range []string{"", "upgrade rejected"} {
			t.Run(fmt.Sprintf("%d/body=%t", status, body != ""), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(status)
					fmt.Fprint(w, body)
				}))
				defer server.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				conn, err := dialWebSocket(ctx, wsURL(server.URL), "expired", nil)
				if conn != nil {
					conn.Close()
				}
				if err == nil {
					t.Fatal("expected the server to reject the upgrade")
				}
				// Both connector retry policy and instance tunnel retry policy must
				// recognize errors produced by the actual WebSocket dialer.
				if got := UpgradeStatus(err.Error()); got != status {
					t.Errorf("status=%d, want %d: %v", got, status, err)
				}
				if got, want := IsNonRetryableError(err.Error()), status >= 400 && status < 500; got != want {
					t.Errorf("non-retryable=%t, want %t: %v", got, want, err)
				}
			})
		}
	}
}

func TestDestinationCreditAndHalfClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	local := make(chan string, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		conn.Write([]byte("abcd"))
		conn.(*net.TCPConn).CloseWrite()
		data, _ := io.ReadAll(conn)
		local <- string(data)
	}()
	complete := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := upgrader.Upgrade(w, r, nil)
		if e != nil {
			complete <- e
			return
		}
		defer ws.Close()
		ws.SetReadDeadline(time.Now().Add(4 * time.Second))
		_, data, e := ws.ReadMessage()
		if e != nil {
			complete <- e
			return
		}
		start, e := DecodeClientMessage(data)
		if e != nil {
			complete <- e
			return
		}
		selectors, e := ParseSelectors(start.SelectorValues, ValidateOptions{})
		if e != nil {
			complete <- e
			return
		}
		hash, _ := ConfigHashWithInspection(selectors, start.Inspection)
		ready, _ := EncodeServerMessage(ServerMessage{Type: "ready", Version: Version, TunnelID: "tunnel-test", ConfigHash: hash, Selectors: SelectorReports(selectors)})
		ws.WriteMessage(websocket.TextMessage, ready)
		open, _ := EncodeServerMessage(ServerMessage{Type: "open", ConnID: 0, SelectorID: "selector-1", Host: "127.0.0.1", Port: port, Window: 2, Transport: TCPTransport()})
		ws.WriteMessage(websocket.TextMessage, open)
		_, data, e = ws.ReadMessage()
		if e != nil {
			complete <- e
			return
		}
		ack, e := DecodeClientMessage(data)
		if e != nil || ack.Type != "openOk" {
			complete <- fmt.Errorf("open ack: %s %v", data, e)
			return
		}
		ws.WriteMessage(websocket.BinaryMessage, encodeMessage(0, []byte("up")))
		fin, _ := EncodeServerMessage(ServerMessage{Type: "fin", ConnID: 0})
		ws.WriteMessage(websocket.TextMessage, fin)
		received := ""
		for {
			kind, data, e := ws.ReadMessage()
			if e != nil {
				complete <- e
				return
			}
			if kind == websocket.BinaryMessage {
				id, payload, e := DecodeDataFrame(data)
				if e != nil || id != 0 || len(payload) > 2 {
					complete <- fmt.Errorf("invalid credit-limited frame %x", data)
					return
				}
				received += string(payload)
				update, _ := EncodeServerMessage(ServerMessage{Type: "windowUpdate", ConnID: 0, Increment: 2})
				ws.WriteMessage(websocket.TextMessage, update)
			} else {
				m, e := DecodeClientMessage(data)
				if e != nil {
					complete <- e
					return
				}
				if m.Type == "fin" {
					if received != "abcd" {
						complete <- fmt.Errorf("data %q", received)
					} else {
						complete <- nil
					}
					return
				}
			}
		}
	}))
	defer server.Close()
	tunnel, err := StartDestinationTCP(ctx, wsURL(server.URL), "token", DestinationOptions{Selectors: []string{fmt.Sprintf("127.0.0.1:%d", port)}})
	if err != nil {
		t.Fatal(err)
	}
	defer tunnel.Close()
	select {
	case err = <-complete:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case got := <-local:
		if got != "up" {
			t.Fatalf("local got %q", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
func TestSingletonReconnectKeepsSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var connections atomic.Int32
	ids := make(chan string, 2)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := upgrader.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		ids <- r.URL.Query().Get("sessionId")
		n := connections.Add(1)
		ws.WriteMessage(websocket.BinaryMessage, []byte(fmt.Sprint(n)))
		if n == 1 {
			return
		}
		for {
			kind, data, e := ws.ReadMessage()
			if e != nil {
				return
			}
			ws.WriteMessage(kind, data)
		}
	}))
	defer server.Close()
	tunnel, err := StartTCP(ctx, wsURL(server.URL), "token", "", 0, TCPOptions{ReconnectDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer tunnel.Close()
	if !tunnel.Address.IP.IsLoopback() {
		t.Fatal(tunnel.Address)
	}
	conn, err := net.Dial("tcp", tunnel.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(4 * time.Second))
	two := make([]byte, 2)
	if _, err = io.ReadFull(conn, two); err != nil {
		t.Fatal(err)
	}
	if string(two) != "12" {
		t.Fatal(string(two))
	}
	first, second := <-ids, <-ids
	if first == "" || first != second {
		t.Fatalf("session IDs %q %q", first, second)
	}
	conn.Write([]byte("roundtrip"))
	data := make([]byte, 9)
	if _, err = io.ReadFull(conn, data); err != nil || string(data) != "roundtrip" {
		t.Fatalf("echo: %q %v", data, err)
	}
	tunnel.Close()
	if _, err = net.DialTimeout("tcp", tunnel.Addr(), 100*time.Millisecond); err == nil {
		t.Fatal("listener remains open")
	}
}
func TestReverseCloseDrainsPendingPayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	received := make(chan string, 1)
	go func() {
		c, e := listener.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		b, _ := io.ReadAll(c)
		received <- string(b)
	}()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := upgrader.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		ws.WriteJSON(map[string]any{"type": "ready", "remoteHost": "127.0.0.1", "remotePort": 57090})
		ws.WriteMessage(websocket.BinaryMessage, encodeMessage(7, []byte("payload before FIN")))
		ws.WriteMessage(websocket.BinaryMessage, EncodeConnectionHeader(7))
		for {
			if _, _, e = ws.ReadMessage(); e != nil {
				return
			}
		}
	}))
	defer server.Close()
	tunnel, e := StartReverseTCP(ctx, wsURL(server.URL), "token", ReverseTCPOptions{LocalPort: listener.Addr().(*net.TCPAddr).Port})
	if e != nil {
		t.Fatal(e)
	}
	defer tunnel.Close()
	select {
	case got := <-received:
		if got != "payload before FIN" {
			t.Fatalf("got %q", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
func TestForwardProxyIsLimitedToConfiguredDestination(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() != "/runner/status?q=1" || r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("upstream %s %v", r.URL, r.Header)
		}
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	proxy, e := StartForwardHTTPProxy(ForwardHTTPProxyOptions{MatchPort: 7001, RemoteBaseURL: upstream.URL + "/runner", Headers: http.Header{"Authorization": []string{"Bearer token"}}})
	if e != nil {
		t.Fatal(e)
	}
	defer proxy.Close()
	for _, tc := range []struct {
		target string
		status int
	}{{"http://127.0.0.1:7001/status?q=1", 200}, {"http://example.com:7001/status", 403}, {"http://127.0.0.1:9999/status", 403}} {
		conn, e := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", proxy.Port))
		if e != nil {
			t.Fatal(e)
		}
		fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: ignored\r\nConnection: close\r\n\r\n", tc.target)
		data, _ := io.ReadAll(conn)
		conn.Close()
		if !strings.HasPrefix(string(data), fmt.Sprintf("HTTP/1.1 %d", tc.status)) {
			t.Fatalf("%s: %s", tc.target, data)
		}
	}
}
func TestInspectionRejectsMalformedFrames(t *testing.T) {
	for _, frame := range []string{`{"type":"body","requestId":"r","data":{"direction":"response","encoding":"base64","chunk":"Zh=="}}`, `{"type":"body","sequence":2,"requestId":"r","data":{"direction":"response","encoding":"base64","chunk":"Zg=="}}`, `{"type":"complete","requestId":"r","data":{}}`} {
		if _, err := DecodeInspectionSSEEvent("1", frame); err == nil {
			t.Fatalf("accepted %s", frame)
		}
	}
	good := map[string]any{"type": "body", "requestId": "r", "data": map[string]any{"direction": "response", "encoding": "base64", "chunk": "Zg=="}}
	raw, _ := json.Marshal(good)
	event, e := DecodeInspectionSSEEvent("1", string(raw))
	if e != nil || string(event.Body) != "f" {
		t.Fatalf("%v %v", event, e)
	}
}
