// Package proxy applies the same environment routing as the TypeScript transport.
package proxy

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gorilla/websocket"
)

func env(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return os.Getenv(strings.ToUpper(name))
}
func nullableEnv(name string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return os.Getenv(strings.ToUpper(name))
}
func bypass(host string, port int, list string, httpFetch bool) bool {
	if list == "*" {
		return true
	}
	host = strings.ToLower(host)
	for _, entry := range strings.FieldsFunc(strings.ToLower(list), func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		target := entry
		if i := strings.LastIndex(entry, ":"); i >= 0 {
			if p, err := strconv.Atoi(entry[i+1:]); err == nil {
				if p != 0 && p != port {
					continue
				}
				target = entry[:i]
			}
		}
		if httpFetch {
			target = strings.TrimPrefix(strings.TrimPrefix(target, "*."), ".")
			if host == target || strings.HasSuffix(host, "."+target) {
				return true
			}
		} else if strings.HasPrefix(target, "*") || strings.HasPrefix(target, ".") {
			if strings.HasSuffix(host, strings.TrimPrefix(target, "*")) {
				return true
			}
		} else if host == target {
			return true
		}
	}
	return false
}

// Destination follows proxy-from-env, used for destination TCP connections.
func Destination(protocol, host string, port int) (*url.URL, error) {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	if bypass(host, port, env("no_proxy"), false) {
		return nil, nil
	}
	raw := env(protocol + "_proxy")
	if raw == "" {
		raw = env("all_proxy")
	}
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = protocol + "://" + raw
	}
	return url.Parse(raw)
}
func port(u *url.URL) int {
	if p, err := strconv.Atoi(u.Port()); err == nil && p != 0 {
		return p
	}
	if u.Scheme == "https" || u.Scheme == "wss" {
		return 443
	}
	return 80
}
func host(u *url.URL) string {
	h := u.Hostname()
	if strings.Contains(h, ":") {
		return "[" + h + "]"
	}
	return h
}
func websocketProxy(r *http.Request) (*url.URL, error) {
	// The TS WebSocket transport only enables environment lookup when an HTTP proxy is configured.
	if env("http_proxy") == "" && env("https_proxy") == "" {
		return nil, nil
	}
	protocol := r.URL.Scheme
	if protocol == "ws" {
		protocol = "http"
	}
	if protocol == "wss" {
		protocol = "https"
	}
	return Destination(protocol, host(r.URL), port(r.URL))
}
func httpProxy(r *http.Request) (*url.URL, error) {
	if bypass(host(r.URL), port(r.URL), nullableEnv("no_proxy"), true) {
		return nil, nil
	}
	raw := nullableEnv("http_proxy")
	if r.URL.Scheme == "https" && nullableEnv("https_proxy") != "" {
		raw = nullableEnv("https_proxy")
	}
	if raw == "" {
		return nil, nil
	}
	return url.Parse(raw)
}

type transport struct {
	base    *http.Transport
	mu      sync.Mutex
	proxies map[string]*http.Transport
}

func (t *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	endpoint, err := httpProxy(r)
	if err != nil {
		return nil, err
	}
	if endpoint == nil {
		return t.base.RoundTrip(r)
	}
	key := endpoint.String()
	t.mu.Lock()
	selected := t.proxies[key]
	if selected == nil {
		selected = t.base.Clone()
		selected.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return Connect(ctx, addr, endpoint, t.base.DialContext)
		}
		// TLS to the destination must be layered over the proxy connection.
		selected.DialTLSContext = nil
		selected.DialTLS = nil //nolint:staticcheck // cleared so the deprecated hook cannot bypass the proxy dial
		t.proxies[key] = selected
	}
	t.mu.Unlock()
	return selected.RoundTrip(r)
}
func (t *transport) CloseIdleConnections() {
	t.base.CloseIdleConnections()
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, proxy := range t.proxies {
		proxy.CloseIdleConnections()
	}
}

var transportsMu sync.Mutex

type transportKey struct {
	base          *http.Transport
	headerTimeout time.Duration
}

var transports = map[transportKey]*transport{}

// Client preserves custom default transports and shares connection pools across SDK helpers.
func Client(headerTimeout ...time.Duration) *http.Client {
	c := *http.DefaultClient
	base := c.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if t, ok := base.(*http.Transport); ok {
		key := transportKey{base: t}
		if len(headerTimeout) > 0 {
			key.headerTimeout = headerTimeout[0]
		}
		transportsMu.Lock()
		cached := transports[key]
		if cached == nil {
			cloned := t.Clone()
			cloned.Proxy = nil
			if key.headerTimeout > 0 {
				cloned.ResponseHeaderTimeout = key.headerTimeout
			}
			cached = &transport{base: cloned, proxies: map[string]*http.Transport{}}
			transports[key] = cached
		}
		transportsMu.Unlock()
		base = cached
	}
	c.Transport = base
	return &c
}
func WebSocketDialer(remote string) *websocket.Dialer {
	d := *websocket.DefaultDialer
	d.Proxy = nil
	forward := d.NetDialContext
	d.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		u, err := url.Parse(remote)
		if err != nil {
			return nil, err
		}
		endpoint, err := websocketProxy(&http.Request{URL: u})
		if err != nil {
			return nil, err
		}
		return Connect(ctx, addr, endpoint, forward)
	}
	return &d
}
