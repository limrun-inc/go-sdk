package limrun_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/limrun-inc/go-sdk/build"
	"github.com/limrun-inc/go-sdk/tunnel"
	"github.com/limrun-inc/go-sdk/websocket/android"
	"github.com/limrun-inc/go-sdk/websocket/ios"
)

func TestWebSocketDialerRejectsInitialConnection(t *testing.T) {
	blocked := errors.New("address rejected by caller")
	for _, scheme := range []string{"http", "https"} {
		for _, platform := range []string{"ios", "android"} {
			t.Run(platform+"/"+scheme, func(t *testing.T) {
				calls := 0
				dialer := &websocket.Dialer{NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					calls++
					port := "80"
					if scheme == "https" {
						port = "443"
					}
					if network != "tcp" || addr != "instance.invalid:"+port || ctx.Err() != nil {
						t.Errorf("unexpected dial: %s %s, context error: %v", network, addr, ctx.Err())
					}
					return nil, blocked
				}}
				var err error
				if platform == "ios" {
					_, err = ios.NewClient(scheme+"://instance.invalid", "test", ios.WithWebSocketDialer(dialer))
				} else {
					_, err = android.NewClient(scheme+"://instance.invalid", "test", android.WithWebSocketDialer(dialer))
				}
				if !errors.Is(err, blocked) || calls != 1 {
					t.Fatalf("error = %v, dial calls = %d", err, calls)
				}
			})
		}
	}
}

func TestWebSocketDialerChecksReconnect(t *testing.T) {
	for _, platform := range []string{"ios", "android"} {
		t.Run(platform, func(t *testing.T) {
			drop := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer ws.Close()
				<-drop
			}))
			defer server.Close()
			var calls atomic.Int32
			rejected := make(chan struct{}, 1)
			dialer := &websocket.Dialer{NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if calls.Add(1) > 1 {
					rejected <- struct{}{}
					return nil, errors.New("address policy changed")
				}
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			}}
			var client io.Closer
			var err error
			if platform == "ios" {
				client, err = ios.NewClient(server.URL, "test", ios.WithWebSocketDialer(dialer), ios.WithReconnect(1, time.Millisecond, time.Millisecond))
			} else {
				client, err = android.NewClient(server.URL, "test", android.WithWebSocketDialer(dialer), android.WithReconnect(1, time.Millisecond, time.Millisecond))
			}
			close(drop)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			select {
			case <-rejected:
			case <-time.After(3 * time.Second):
				t.Fatal("reconnect bypassed the custom dialer")
			}
		})
	}
}

func TestWebSocketDialerInheritedByStreamsAndTunnels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	blocked := errors.New("address rejected by caller")
	var reject atomic.Bool
	var calls atomic.Int32
	dialer := &websocket.Dialer{NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		calls.Add(1)
		if reject.Load() {
			return nil, blocked
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	iosClient, err := ios.NewClient(server.URL, "test", ios.WithWebSocketDialer(dialer))
	if err != nil {
		t.Fatal(err)
	}
	defer iosClient.Close()
	androidClient, err := android.NewClient(server.URL, "test", android.WithWebSocketDialer(dialer), android.WithADBURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	defer androidClient.Close()
	buildClient := build.NewXcodeClient(server.URL, "test", build.WithWebSocketDialer(dialer))
	reject.Store(true)
	for _, tc := range []struct {
		name string
		open func(context.Context) (io.Closer, error)
	}{
		{"ios syslog", func(ctx context.Context) (io.Closer, error) {
			stream, err := iosClient.StreamSyslog(ctx)
			if err == nil {
				stream.Stop()
			}
			return nil, err
		}},
		{"ios app log", func(ctx context.Context) (io.Closer, error) {
			stream, err := iosClient.StreamAppLog(ctx, "com.example.app")
			if err == nil {
				stream.Stop()
			}
			return nil, err
		}},
		{"ios destination", func(ctx context.Context) (io.Closer, error) {
			return iosClient.StartTunnel(ctx, ios.TunnelOptions{Selectors: []string{"localhost:8080"}})
		}},
		{"android destination", func(ctx context.Context) (io.Closer, error) {
			return androidClient.StartTunnel(ctx, android.TunnelOptions{Selectors: []string{"localhost:8080"}})
		}},
		{"ios reverse", func(ctx context.Context) (io.Closer, error) {
			return iosClient.StartReverseTunnel(ctx, ios.ReverseTunnelOptions{RemotePort: 57090})
		}},
		{"xcode rbe", func(ctx context.Context) (io.Closer, error) {
			port := 0
			return buildClient.StartRbeTunnel(ctx, build.RBETunnelOptions{Port: &port})
		}},
		{"tcp", func(ctx context.Context) (io.Closer, error) {
			return tunnel.StartTCP(ctx, "ws://instance.invalid", "test", "127.0.0.1", 0, tunnel.TCPOptions{Mode: "multiplexed", WebSocketDialer: dialer})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			before := calls.Load()
			opened, err := tc.open(ctx)
			if err == nil && opened != nil {
				defer opened.Close()
			}
			if !errors.Is(err, blocked) || calls.Load() != before+1 {
				t.Fatalf("error = %v, dial calls = %d", err, calls.Load()-before)
			}
		})
	}
}
