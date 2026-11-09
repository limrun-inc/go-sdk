package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"http_proxy", "https_proxy", "no_proxy", "all_proxy", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY"} {
		t.Setenv(name, "")
	}
}
func TestProxyEnvironmentMatchesTypeScript(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("HTTP_PROXY", "http://upper.example:80")
	t.Setenv("http_proxy", "http://lower.example:80")
	t.Setenv("all_proxy", "http://fallback.example:80")
	t.Setenv("no_proxy", "example.com,.local.test,*suffix.test,localhost:4000,[::1]:5000")
	for _, tc := range []struct {
		raw              string
		httpWant, wsWant string
	}{
		{"http://example.com", "", ""},
		{"http://sub.example.com", "", "http://lower.example:80"},
		{"http://a.local.test", "", ""},
		{"http://local.test", "", "http://lower.example:80"},
		{"http://asuffix.test", "http://lower.example:80", ""},
		{"http://localhost:4000", "", ""},
		{"http://localhost:4001", "http://lower.example:80", "http://lower.example:80"},
		{"http://[::1]:5000", "", ""},
		{"https://other.test", "http://lower.example:80", "http://fallback.example:80"},
	} {
		u, _ := url.Parse(tc.raw)
		for _, engine := range []struct {
			name string
			fn   func(*http.Request) (*url.URL, error)
			want string
		}{{"http", httpProxy, tc.httpWant}, {"ws", websocketProxy, tc.wsWant}} {
			got, err := engine.fn(&http.Request{URL: u})
			if err != nil {
				t.Fatal(err)
			}
			value := ""
			if got != nil {
				value = got.String()
			}
			if value != engine.want {
				t.Errorf("%s %s = %q, want %q", engine.name, tc.raw, value, engine.want)
			}
		}
	}
}
func TestHTTPAndWebSocketUseAuthenticatedCONNECT(t *testing.T) {
	clearProxyEnv(t)
	upgrade := websocket.Upgrader{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws" {
			ws, e := upgrade.Upgrade(w, r, nil)
			if e != nil {
				return
			}
			defer ws.Close()
			kind, b, e := ws.ReadMessage()
			if e == nil {
				_ = ws.WriteMessage(kind, b)
			}
			return
		}
		_, _ = io.WriteString(w, "through tunnel")
	}))
	defer target.Close()
	requests := make(chan string, 4)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			t.Errorf("proxy method=%s", r.Method)
			w.WriteHeader(400)
			return
		}
		if r.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
			t.Error("proxy auth missing")
		}
		peer, e := net.Dial("tcp", r.Host)
		if e != nil {
			t.Error(e)
			w.WriteHeader(502)
			return
		}
		conn, rw, e := w.(http.Hijacker).Hijack()
		if e != nil {
			peer.Close()
			t.Error(e)
			return
		}
		defer conn.Close()
		defer peer.Close()
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		requests <- r.Host
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(peer, rw)
			if c, ok := peer.(*net.TCPConn); ok {
				_ = c.CloseWrite()
			}
			close(done)
		}()
		_, _ = io.Copy(conn, peer)
		_ = conn.Close()
		<-done
	}))
	defer proxyServer.Close()
	t.Setenv("http_proxy", strings.Replace(proxyServer.URL, "http://", "http://user:pass@", 1))
	client := Client()
	defer client.CloseIdleConnections()
	res, e := client.Get(target.URL)
	if e != nil {
		t.Fatal(e)
	}
	body, e := io.ReadAll(res.Body)
	res.Body.Close()
	if e != nil || string(body) != "through tunnel" {
		t.Fatalf("body=%q err=%v", body, e)
	}
	endpoint := strings.Replace(target.URL, "http://", "ws://", 1) + "/ws"
	ws, _, e := WebSocketDialer(endpoint).DialContext(context.Background(), endpoint, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = ws.WriteMessage(websocket.BinaryMessage, []byte("echo")); e != nil {
		t.Fatal(e)
	}
	_, body, e = ws.ReadMessage()
	ws.Close()
	if e != nil || string(body) != "echo" {
		t.Fatalf("echo=%q err=%v", body, e)
	}
	if len(requests) != 2 {
		t.Fatalf("CONNECT count=%d", len(requests))
	}
}
