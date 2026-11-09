// Package control implements the shared simulator command transport.
package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/limrun-inc/go-sdk/internal/proxy"

	"github.com/gorilla/websocket"
)

var ErrNotConnected = errors.New("websocket: not connected")
var ErrClosed = errors.New("websocket: connection closed")

type State string

const (
	Connecting   State = "connecting"
	Connected    State = "connected"
	Disconnected State = "disconnected"
	Reconnecting State = "reconnecting"
)

type Options struct {
	MaxReconnectAttempts  int
	ReconnectDelay        time.Duration
	MaxReconnectDelay     time.Duration
	Dialer                *websocket.Dialer
	OnMessage             func(json.RawMessage) bool
	OnDisconnect          func(error)
	OnPermanentDisconnect func()
}
type reply struct {
	data json.RawMessage
	err  error
}
type Client struct {
	mu        sync.Mutex
	writeMu   sync.Mutex
	ws        *websocket.Conn
	endpoint  string
	opts      Options
	state     State
	pending   map[string]chan reply
	listeners map[string]func(State)
	ctx       context.Context
	cancel    context.CancelFunc
	closed    bool
}

func ID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprint(time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
func URL(apiURL, token, path string) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("invalid API URL scheme: %s", u.Scheme)
	}
	u = u.JoinPath(path)
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func New(ctx context.Context, endpoint string, opts Options) (*Client, error) {
	if opts.Dialer == nil {
		opts.Dialer = proxy.WebSocketDialer(endpoint)
	}
	if opts.ReconnectDelay == 0 {
		opts.ReconnectDelay = time.Second
	}
	if opts.MaxReconnectDelay == 0 {
		opts.MaxReconnectDelay = 30 * time.Second
	}
	lifetime, cancel := context.WithCancel(context.Background())
	c := &Client{endpoint: endpoint, opts: opts, state: Connecting, pending: make(map[string]chan reply), listeners: make(map[string]func(State)), ctx: lifetime, cancel: cancel}
	ws, res, err := opts.Dialer.DialContext(ctx, endpoint, http.Header{})
	if err != nil {
		cancel()
		if res != nil && res.Body != nil {
			res.Body.Close()
		}
		return nil, err
	}
	c.ws = ws
	c.state = Connected
	go c.run(ws)
	return c, nil
}
func (c *Client) State() State { c.mu.Lock(); defer c.mu.Unlock(); return c.state }
func (c *Client) OnStateChange(fn func(State)) func() {
	id := ID()
	c.mu.Lock()
	c.listeners[id] = fn
	c.mu.Unlock()
	return func() { c.mu.Lock(); delete(c.listeners, id); c.mu.Unlock() }
}
func (c *Client) setState(s State) {
	c.mu.Lock()
	if c.state == s {
		c.mu.Unlock()
		return
	}
	c.state = s
	fs := make([]func(State), 0, len(c.listeners))
	for _, fn := range c.listeners {
		fs = append(fs, fn)
	}
	c.mu.Unlock()
	for _, fn := range fs {
		func() { defer func() { _ = recover() }(); fn(s) }()
	}
}
func (c *Client) fail(err error) {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[string]chan reply)
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- reply{err: err}
	}
	if c.opts.OnDisconnect != nil {
		c.opts.OnDisconnect(err)
	}
}
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.cancel()
	ws := c.ws
	c.ws = nil
	c.mu.Unlock()
	var err error
	if ws != nil {
		err = ws.Close()
	}
	c.fail(ErrClosed)
	c.setState(Disconnected)
	return err
}
func (c *Client) Send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	ws := c.ws
	closed := c.closed
	c.mu.Unlock()
	if ws == nil || closed {
		return ErrNotConnected
	}
	return ws.WriteMessage(websocket.TextMessage, data)
}
func (c *Client) Call(ctx context.Context, kind string, params any, timeout time.Duration) (json.RawMessage, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	fields := map[string]any{}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &fields); err != nil {
			return nil, err
		}
	}
	id := ID()
	fields["id"] = id
	fields["type"] = kind
	ch := make(chan reply, 1)
	c.mu.Lock()
	if c.closed || c.ws == nil {
		c.mu.Unlock()
		return nil, ErrNotConnected
	}
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	if err := c.Send(fields); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		var e struct {
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(r.data, &e); err != nil {
			return nil, err
		}
		if len(e.Error) > 0 && string(e.Error) != "null" {
			var message string
			if json.Unmarshal(e.Error, &message) != nil {
				var obj struct {
					Message string `json:"message"`
				}
				if json.Unmarshal(e.Error, &obj) == nil {
					message = obj.Message
				}
			}
			if message != "" {
				return nil, errors.New(message)
			}
		}
		return r.data, nil
	}
}
func (c *Client) read(ws *websocket.Conn) error {
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-c.ctx.Done():
				return
			case <-t.C:
				if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					ws.Close()
					return
				}
			}
		}
	}()
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		if c.opts.OnMessage != nil && c.opts.OnMessage(data) {
			continue
		}
		var msg struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		c.mu.Lock()
		ch := c.pending[msg.ID]
		delete(c.pending, msg.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- reply{data: data}
		}
	}
}
func (c *Client) run(ws *websocket.Conn) {
	for {
		err := c.read(ws)
		ws.Close()
		c.mu.Lock()
		if c.ws == ws {
			c.ws = nil
		}
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return
		}
		c.fail(err)
		c.setState(Disconnected)
		reconnected := false
		for attempt := 0; attempt < c.opts.MaxReconnectAttempts; attempt++ {
			c.setState(Reconnecting)
			delay := c.opts.ReconnectDelay
			for i := 0; i < attempt && delay < c.opts.MaxReconnectDelay; i++ {
				delay *= 2
			}
			if delay > c.opts.MaxReconnectDelay {
				delay = c.opts.MaxReconnectDelay
			}
			timer := time.NewTimer(delay)
			select {
			case <-c.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			next, res, dialErr := c.opts.Dialer.DialContext(c.ctx, c.endpoint, http.Header{})
			if dialErr != nil {
				if res != nil {
					if res.Body != nil {
						res.Body.Close()
					}
					if res.StatusCode >= 400 && res.StatusCode < 500 {
						break
					}
				}
				if IsNonRetryableError(dialErr.Error()) {
					break
				}
				continue
			}
			c.mu.Lock()
			if c.closed {
				c.mu.Unlock()
				next.Close()
				return
			}
			c.ws = next
			c.mu.Unlock()
			ws = next
			reconnected = true
			c.setState(Connected)
			break
		}
		if !reconnected {
			if c.opts.OnPermanentDisconnect != nil {
				c.opts.OnPermanentDisconnect()
			}
			c.setState(Disconnected)
			return
		}
	}
}

var upgradeStatus = regexp.MustCompile(`(?i)(?:unexpected server response:|unexpected status(?: code)?[: ]|http[ /][0-9.]+)\s*(\d{3})`)

func IsNonRetryableError(s string) bool {
	m := upgradeStatus.FindStringSubmatch(s)
	if len(m) < 2 {
		return false
	}
	status, _ := strconv.Atoi(m[1])
	return status >= 400 && status < 500
}
