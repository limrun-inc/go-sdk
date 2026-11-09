package tunnel

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/limrun-inc/go-sdk/internal/proxy"

	"github.com/gorilla/websocket"
	"github.com/limrun-inc/go-sdk/internal/control"
)

type ConnectionState string

const (
	Connecting   ConnectionState = "connecting"
	Connected    ConnectionState = "connected"
	Disconnected ConnectionState = "disconnected"
	Reconnecting ConnectionState = "reconnecting"
)

type connectionState struct {
	mu        sync.Mutex
	value     ConnectionState
	next      uint64
	callbacks map[uint64]func(ConnectionState)
}

func (s *connectionState) get() ConnectionState { s.mu.Lock(); defer s.mu.Unlock(); return s.value }
func (s *connectionState) set(value ConnectionState) {
	s.mu.Lock()
	if s.value == value {
		s.mu.Unlock()
		return
	}
	s.value = value
	callbacks := make([]func(ConnectionState), 0, len(s.callbacks))
	for _, f := range s.callbacks {
		callbacks = append(callbacks, f)
	}
	s.mu.Unlock()
	for _, f := range callbacks {
		func() { defer func() { _ = recover() }(); f(value) }()
	}
}
func (s *connectionState) subscribe(f func(ConnectionState)) func() {
	s.mu.Lock()
	if s.callbacks == nil {
		s.callbacks = map[uint64]func(ConnectionState){}
	}
	s.next++
	id := s.next
	s.callbacks[id] = f
	s.mu.Unlock()
	return func() { s.mu.Lock(); delete(s.callbacks, id); s.mu.Unlock() }
}

type TCPOptions struct {
	Mode                              string
	MaxReconnectAttempts              *int
	ReconnectDelay, MaxReconnectDelay time.Duration
}

func (o TCPOptions) attempts() int {
	if o.MaxReconnectAttempts != nil {
		return *o.MaxReconnectAttempts
	}
	return 6
}
func (o TCPOptions) delay(attempt int) time.Duration {
	d := o.ReconnectDelay
	if d == 0 {
		d = time.Second
	}
	max := o.MaxReconnectDelay
	if max == 0 {
		max = 30 * time.Second
	}
	for i := 0; i < attempt && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}
func wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func AssertPort(port int, name string, min, max int) error {
	if port < min || port > max {
		return fmt.Errorf("%s must be an integer between %d and %d", name, min, max)
	}
	return nil
}
func EncodeConnectionHeader(id uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, id)
	return b
}
func DecodeConnectionHeader(b []byte) (uint32, error) {
	if len(b) < 4 {
		return 0, errors.New("header must be at least 4 bytes")
	}
	return binary.BigEndian.Uint32(b), nil
}
func encodeMessage(id uint32, data []byte) []byte {
	b := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(b, id)
	copy(b[4:], data)
	return b
}
func decodeMessage(b []byte) (uint32, []byte, error) {
	id, e := DecodeConnectionHeader(b)
	if e != nil {
		return 0, nil, e
	}
	return id, b[4:], nil
}
func dialWebSocket(ctx context.Context, remote, token string) (*websocket.Conn, error) {
	ws, res, err := proxy.WebSocketDialer(remote).DialContext(ctx, remote, http.Header{"Authorization": []string{"Bearer " + token}})
	if err != nil && res != nil {
		var body string
		if res.Body != nil {
			data, _ := io.ReadAll(io.LimitReader(res.Body, 201))
			body = strings.TrimSpace(string(data))
			res.Body.Close()
		}
		if len(body) > 200 {
			body = body[:200]
		}
		if body != "" {
			return nil, fmt.Errorf("unexpected server response: %d: %s: %w", res.StatusCode, body, err)
		}
		return nil, fmt.Errorf("unexpected server response: %d: %w", res.StatusCode, err)
	}
	return ws, err
}
func IsNonRetryableError(message string) bool {
	return control.IsNonRetryableError(message)
}
func UpgradeStatus(message string) int {
	var code int
	for i := 0; i < len(message); i++ {
		if _, e := fmt.Sscanf(message[i:], "Unexpected server response: %d", &code); e == nil {
			return code
		}
	}
	return 0
}
func pingWebSocket(ctx context.Context, ws *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)) != nil {
				ws.Close()
				return
			}
		}
	}
}

type TCP struct {
	Address       *net.TCPAddr
	listener      net.Listener
	ctx           context.Context
	cancel        context.CancelFunc
	once          sync.Once
	state         connectionState
	mu            sync.Mutex
	writeMu       sync.Mutex
	ws            *websocket.Conn
	connections   map[uint32]*tcpPeer
	nextID        uint32
	changed       chan struct{}
	remote, token string
	opts          TCPOptions
}
type tcpPeer struct {
	ending bool
	conn   net.Conn
	mu     sync.Mutex
	queue  [][]byte
	bytes  int
	closed bool
	wake   chan struct{}
	cancel context.CancelFunc
}

func newPeer(conn net.Conn) *tcpPeer { return &tcpPeer{conn: conn, wake: make(chan struct{}, 1)} }
func (p *tcpPeer) push(data []byte, limit int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.ending || limit > 0 && p.bytes+len(data) > limit {
		return false
	}
	p.queue = append(p.queue, append([]byte(nil), data...))
	p.bytes += len(data)
	select {
	case p.wake <- struct{}{}:
	default:
	}
	return true
}
func (p *tcpPeer) end() {
	p.mu.Lock()
	p.ending = true
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}
func (p *tcpPeer) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	c := p.conn
	cancel := p.cancel
	p.queue = nil
	p.bytes = 0
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if c != nil {
		c.Close()
	}
	select {
	case p.wake <- struct{}{}:
	default:
	}
}
func (p *tcpPeer) flush(ctx context.Context) error {
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return net.ErrClosed
		}
		if len(p.queue) > 0 {
			data := p.queue[0]
			p.queue = p.queue[1:]
			p.bytes -= len(data)
			conn := p.conn
			p.mu.Unlock()
			if _, err := conn.Write(data); err != nil {
				return err
			}
			continue
		}
		if p.ending {
			p.mu.Unlock()
			return io.EOF
		}
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.wake:
		}
	}
}
func StartTCP(ctx context.Context, remote, token, host string, port int, options ...TCPOptions) (*TCP, error) {
	if err := AssertPort(port, "port", 0, 65535); err != nil {
		return nil, err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	var opts TCPOptions
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.Mode != "" && opts.Mode != "singleton" && opts.Mode != "multiplexed" {
		listener.Close()
		return nil, errors.New("invalid tunnel mode")
	}
	return startTCPListener(ctx, listener, remote, token, opts)
}
func startTCPListener(ctx context.Context, listener net.Listener, remote, token string, opts TCPOptions) (*TCP, error) {
	ctx, cancel := context.WithCancel(ctx)
	t := &TCP{Address: listener.Addr().(*net.TCPAddr), listener: listener, ctx: ctx, cancel: cancel, connections: map[uint32]*tcpPeer{}, changed: make(chan struct{}), remote: remote, token: token, opts: opts, state: connectionState{value: Connecting}}
	if opts.Mode == "multiplexed" {
		u, err := url.Parse(remote)
		if err != nil {
			t.Close()
			return nil, err
		}
		q := u.Query()
		q.Set("mode", "multiplexed")
		u.RawQuery = q.Encode()
		t.remote = u.String()
		ws, err := dialWebSocket(ctx, t.remote, token)
		if err != nil {
			t.Close()
			return nil, err
		}
		t.setWS(ws)
		go t.multiplexed(ws)
	}
	go t.accept()
	go func() { <-ctx.Done(); t.Close() }()
	return t, nil
}
func (t *TCP) Addr() string                                           { return t.listener.Addr().String() }
func (t *TCP) GetConnectionState() ConnectionState                    { return t.state.get() }
func (t *TCP) OnConnectionStateChange(f func(ConnectionState)) func() { return t.state.subscribe(f) }
func (t *TCP) setWS(ws *websocket.Conn) {
	t.mu.Lock()
	if ws != nil && t.ctx.Err() != nil {
		t.mu.Unlock()
		ws.Close()
		return
	}
	t.ws = ws
	close(t.changed)
	t.changed = make(chan struct{})
	t.mu.Unlock()
	if ws != nil {
		t.state.set(Connected)
	} else {
		t.state.set(Disconnected)
	}
}
func (t *TCP) Close() error {
	changed := false
	t.once.Do(func() {
		changed = true
		t.cancel()
		t.listener.Close()
		t.mu.Lock()
		ws := t.ws
		t.ws = nil
		peers := t.connections
		t.connections = map[uint32]*tcpPeer{}
		close(t.changed)
		t.changed = make(chan struct{})
		t.mu.Unlock()
		if ws != nil {
			ws.Close()
		}
		for _, p := range peers {
			p.close()
		}
	})
	if changed {
		t.state.set(Disconnected)
	}
	return nil
}
func (t *TCP) send(ws *websocket.Conn, kind int, data []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return ws.WriteMessage(kind, data)
}
func (t *TCP) remove(id uint32, p *tcpPeer, ws *websocket.Conn, notify bool) {
	t.mu.Lock()
	current := t.connections[id] == p
	if current {
		delete(t.connections, id)
	}
	active := t.ws == ws
	t.mu.Unlock()
	p.close()
	if current && notify && active && t.ctx.Err() == nil {
		_ = t.send(ws, websocket.BinaryMessage, EncodeConnectionHeader(id))
	}
}
func (t *TCP) accept() {
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			return
		}
		p := newPeer(conn)
		t.mu.Lock()
		ws := t.ws
		if t.ctx.Err() != nil || t.opts.Mode == "multiplexed" && ws == nil || t.opts.Mode != "multiplexed" && len(t.connections) > 0 {
			t.mu.Unlock()
			conn.Close()
			continue
		}
		t.nextID++
		if t.nextID == 0 {
			t.nextID++
		}
		id := t.nextID
		t.connections[id] = p
		t.mu.Unlock()
		if t.opts.Mode == "multiplexed" {
			go t.forwardPeer(id, p, ws)
		} else {
			go t.singleton(id, p)
		}
	}
}
func (t *TCP) forwardPeer(id uint32, p *tcpPeer, ws *websocket.Conn) {
	go func() { _ = p.flush(t.ctx); t.remove(id, p, ws, true) }()
	buf := make([]byte, 32*1024)
	for {
		n, err := p.conn.Read(buf)
		if n > 0 {
			if e := t.send(ws, websocket.BinaryMessage, encodeMessage(id, buf[:n])); e != nil {
				ws.Close()
				break
			}
		}
		if err != nil {
			break
		}
	}
	t.remove(id, p, ws, true)
}
func (t *TCP) multiplexed(ws *websocket.Conn) {
	attempt := 0
	for {
		ctx, cancel := context.WithCancel(t.ctx)
		go pingWebSocket(ctx, ws)
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				break
			}
			id, payload, e := decodeMessage(data)
			if e != nil {
				continue
			}
			t.mu.Lock()
			p := t.connections[id]
			t.mu.Unlock()
			if p == nil {
				continue
			}
			if len(payload) == 0 {
				t.remove(id, p, ws, false)
			} else if !p.push(payload, 0) {
				t.remove(id, p, ws, true)
			}
		}
		cancel()
		ws.Close()
		t.setWS(nil)
		t.mu.Lock()
		peers := t.connections
		t.connections = map[uint32]*tcpPeer{}
		t.mu.Unlock()
		for _, p := range peers {
			p.close()
		}
		for {
			if t.ctx.Err() != nil || attempt >= t.opts.attempts() {
				t.Close()
				return
			}
			t.state.set(Reconnecting)
			if !wait(t.ctx, t.opts.delay(attempt)) {
				return
			}
			attempt++
			t.state.set(Connecting)
			var err error
			ws, err = dialWebSocket(t.ctx, t.remote, t.token)
			if err == nil {
				attempt = 0
				t.setWS(ws)
				break
			}
			if IsNonRetryableError(err.Error()) {
				t.Close()
				return
			}
		}
	}
}
func (t *TCP) singleton(id uint32, p *tcpPeer) {
	ctx, cancel := context.WithCancel(t.ctx)
	defer cancel()
	defer func() { t.mu.Lock(); delete(t.connections, id); t.mu.Unlock(); p.close(); t.state.set(Disconnected) }()
	u, err := url.Parse(t.remote)
	if err != nil {
		return
	}
	q := u.Query()
	q.Set("sessionId", control.ID())
	u.RawQuery = q.Encode()
	// Keep the TCP connection and session ID through WebSocket reconnects.
	go func() {
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			t.mu.Lock()
			ws, changed := t.ws, t.changed
			t.mu.Unlock()
			if ws == nil {
				select {
				case <-ctx.Done():
					return
				case <-changed:
					continue
				}
			}
			n, e := p.conn.Read(buf)
			if n > 0 {
				for {
					t.mu.Lock()
					ws, changed = t.ws, t.changed
					t.mu.Unlock()
					if ws != nil {
						if t.send(ws, websocket.BinaryMessage, buf[:n]) != nil {
							ws.Close()
						}
						break
					}
					select {
					case <-ctx.Done():
						return
					case <-changed:
					}
				}
			}
			if e != nil {
				return
			}
		}
	}()
	go func() { _ = p.flush(ctx); cancel() }()
	attempt := 0
	for ctx.Err() == nil {
		t.state.set(Connecting)
		ws, e := dialWebSocket(ctx, u.String(), t.token)
		if e == nil {
			attempt = 0
			t.setWS(ws)
			connectionCtx, stop := context.WithCancel(ctx)
			go pingWebSocket(connectionCtx, ws)
			go func() { <-connectionCtx.Done(); ws.Close() }()
			for {
				_, data, readErr := ws.ReadMessage()
				if readErr != nil {
					break
				}
				if !p.push(data, 0) {
					break
				}
			}
			stop()
			ws.Close()
			t.setWS(nil)
		} else if IsNonRetryableError(e.Error()) {
			t.Close()
			return
		}
		if ctx.Err() != nil {
			return
		}
		if attempt >= t.opts.attempts() {
			t.Close()
			return
		}
		t.state.set(Reconnecting)
		if !wait(ctx, t.opts.delay(attempt)) {
			return
		}
		attempt++
	}
}

type ReverseTCPOptions struct {
	LocalHost                                    string
	LocalPort                                    int
	MaxConnections, MaxPendingBytesPerConnection int
	// MaxBufferedBytes is retained for TypeScript option compatibility. Go writes
	// block the producer until the WebSocket accepts data, without an async queue.
	MaxBufferedBytes int
	ConnectTimeout   time.Duration
}
type ReverseTCP struct {
	RemoteAddress *net.TCPAddr
	remoteHost    string
	ctx           context.Context
	cancel        context.CancelFunc
	once          sync.Once
	state         connectionState
	ws            *websocket.Conn
	writeMu       sync.Mutex
	mu            sync.Mutex
	peers         map[uint32]*tcpPeer
	closedIDs     map[uint32]time.Time
	opts          ReverseTCPOptions
}

func StartReverseTCP(ctx context.Context, remote, token string, opts ReverseTCPOptions) (*ReverseTCP, error) {
	if err := AssertPort(opts.LocalPort, "localPort", 1, 65535); err != nil {
		return nil, err
	}
	if opts.LocalHost == "" {
		opts.LocalHost = "127.0.0.1"
	}
	if opts.MaxConnections == 0 {
		opts.MaxConnections = 64
	}
	if opts.MaxPendingBytesPerConnection == 0 {
		opts.MaxPendingBytesPerConnection = 16 * 1024 * 1024
	}
	if opts.MaxBufferedBytes == 0 {
		opts.MaxBufferedBytes = 4 * 1024 * 1024
	}
	if opts.ConnectTimeout == 0 {
		opts.ConnectTimeout = 10 * time.Second
	}
	ctx, cancel := context.WithCancel(ctx)
	ws, err := dialWebSocket(ctx, remote, token)
	if err != nil {
		cancel()
		return nil, err
	}
	t := &ReverseTCP{ctx: ctx, cancel: cancel, ws: ws, peers: map[uint32]*tcpPeer{}, closedIDs: map[uint32]time.Time{}, opts: opts, state: connectionState{value: Connecting}}
	go func() { <-ctx.Done(); t.Close() }()
	go pingWebSocket(ctx, ws)
	kind, data, err := ws.ReadMessage()
	if err != nil {
		t.Close()
		return nil, err
	}
	var ready struct {
		Type    string `json:"type"`
		Host    string `json:"remoteHost"`
		Port    *int   `json:"remotePort"`
		Message string `json:"message"`
	}
	if err = json.Unmarshal(data, &ready); err != nil || kind != websocket.TextMessage || ready.Type != "ready" || ready.Host == "" || ready.Port == nil {
		t.Close()
		if ready.Message != "" {
			return nil, errors.New(ready.Message)
		}
		return nil, fmt.Errorf("malformed reverse tunnel ready message: %s", data)
	}
	t.remoteHost = ready.Host
	t.RemoteAddress = &net.TCPAddr{IP: net.ParseIP(ready.Host), Port: *ready.Port}
	t.state.set(Connected)
	go t.read()
	return t, nil
}
func (t *ReverseTCP) RemoteAddr() string {
	return net.JoinHostPort(t.remoteHost, strconv.Itoa(t.RemoteAddress.Port))
}
func (t *ReverseTCP) GetConnectionState() ConnectionState { return t.state.get() }
func (t *ReverseTCP) OnConnectionStateChange(f func(ConnectionState)) func() {
	return t.state.subscribe(f)
}
func (t *ReverseTCP) Close() error {
	changed := false
	t.once.Do(func() {
		changed = true
		t.cancel()
		t.ws.Close()
		t.mu.Lock()
		peers := t.peers
		t.peers = map[uint32]*tcpPeer{}
		t.mu.Unlock()
		for _, p := range peers {
			p.close()
		}
	})
	if changed {
		t.state.set(Disconnected)
	}
	return nil
}
func (t *ReverseTCP) send(id uint32, data []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.ws.WriteMessage(websocket.BinaryMessage, encodeMessage(id, data))
}
func (t *ReverseTCP) remove(id uint32, notify bool) {
	t.mu.Lock()
	p := t.peers[id]
	delete(t.peers, id)
	t.closedIDs[id] = time.Now().Add(30 * time.Second)
	t.mu.Unlock()
	if p != nil {
		p.close()
	}
	if notify && t.ctx.Err() == nil {
		_ = t.send(id, nil)
	}
}
func (t *ReverseTCP) read() {
	defer t.Close()
	for {
		kind, data, err := t.ws.ReadMessage()
		if err != nil {
			return
		}
		if kind != websocket.BinaryMessage {
			continue
		}
		id, payload, err := decodeMessage(data)
		if err != nil {
			continue
		}
		if len(payload) == 0 {
			t.mu.Lock()
			p := t.peers[id]
			t.closedIDs[id] = time.Now().Add(30 * time.Second)
			t.mu.Unlock()
			if p != nil {
				p.end()
			}
			continue
		}
		t.mu.Lock()
		now := time.Now()
		for id, until := range t.closedIDs {
			if !now.Before(until) {
				delete(t.closedIDs, id)
			}
		}
		_, closed := t.closedIDs[id]
		p := t.peers[id]
		if closed {
			t.mu.Unlock()
			continue
		}
		fresh := p == nil
		if fresh {
			if len(t.peers) >= t.opts.MaxConnections {
				t.mu.Unlock()
				t.remove(id, true)
				continue
			}
			p = newPeer(nil)
			t.peers[id] = p
		}
		t.mu.Unlock()
		if !p.push(payload, t.opts.MaxPendingBytesPerConnection) {
			t.remove(id, true)
			continue
		}
		if fresh {
			go t.connect(id, p)
		}
	}
}
func (t *ReverseTCP) connect(id uint32, p *tcpPeer) {
	ctx, cancel := context.WithTimeout(t.ctx, t.opts.ConnectTimeout)
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		cancel()
		return
	}
	p.cancel = cancel
	p.mu.Unlock()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(t.opts.LocalHost, strconv.Itoa(t.opts.LocalPort)))
	cancel()
	if err != nil {
		t.remove(id, true)
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		conn.Close()
		return
	}
	p.conn = conn
	p.mu.Unlock()
	go func() {
		err := p.flush(t.ctx)
		if err == io.EOF {
			if c, ok := p.conn.(interface{ CloseWrite() error }); ok {
				_ = c.CloseWrite()
			}
			wait(t.ctx, time.Second)
		}
		t.remove(id, true)
	}()
	buf := make([]byte, 32*1024)
	for {
		n, e := conn.Read(buf)
		if n > 0 {
			if err = t.send(id, buf[:n]); err != nil {
				t.Close()
				return
			}
		}
		if e != nil {
			break
		}
	}
	t.remove(id, true)
}
