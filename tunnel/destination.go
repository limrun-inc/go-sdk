package tunnel

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type DestinationSessionError struct{ Code string }

func (e *DestinationSessionError) Error() string { return "destination tunnel failed: " + e.Code }
func IsTerminalDestinationTunnelError(err error) bool {
	var session *DestinationSessionError
	return errors.As(err, &session)
}

type DestinationOptions struct {
	Selectors                                                          []string
	Inspection                                                         *InspectionConfig
	OnInspectionEvent                                                  func(InspectionEvent)
	OnInspectionError                                                  func(error)
	Window                                                             int
	Name                                                               string
	ResetsInactivityTimeout                                            *bool
	MaxConnections, MaxPendingBytesPerConnection, MaxTotalPendingBytes int
	// MaxBufferedBytes is retained for TypeScript option compatibility. Go writes
	// block the producer until the WebSocket accepts data, without an async queue.
	MaxBufferedBytes                                  int
	ConnectTimeout, HandshakeTimeout, LivenessTimeout time.Duration
}
type Destination struct {
	TunnelID         string
	Selectors        []SelectorReport
	ConfigHash       string
	Inspection       InspectionConfig
	InspectionStream *InspectionStream
	state            connectionState
	ctx              context.Context
	cancel           context.CancelFunc
	ws               *websocket.Conn
	writeMu          sync.Mutex
	mu               sync.Mutex
	once             sync.Once
	flows            map[uint32]*destinationFlow
	recentlyClosed   map[uint32]time.Time
	pendingBytes     int
	err              error
	done             chan struct{}
	opts             DestinationOptions
	routes           []Route
}
type destinationFlow struct {
	conn                                     net.Conn
	cancel                                   context.CancelFunc
	ctx                                      context.Context
	credit                                   int64
	pending                                  [][]byte
	pendingBytes                             int
	delivered                                int
	open, localFIN, remoteFIN, remoteDrained bool
	wake, creditWake                         chan struct{}
}

func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
func StartDestinationTCP(ctx context.Context, remote, token string, opts DestinationOptions) (*Destination, error) {
	selectors, err := ParseSelectors(opts.Selectors, ValidateOptions{})
	if err != nil {
		return nil, err
	}
	opts.Selectors = FormatSelectors(selectors)
	inspection := DisabledInspection()
	if opts.Inspection != nil {
		inspection, err = NormalizeInspectionConfig(*opts.Inspection)
		if err != nil {
			return nil, err
		}
	}
	defaults := []struct {
		value    *int
		fallback int
		name     string
	}{{&opts.Window, DefaultWindow, "window"}, {&opts.MaxConnections, 64, "maxConnections"}, {&opts.MaxPendingBytesPerConnection, 16 * 1024 * 1024, "maxPendingBytesPerConnection"}, {&opts.MaxTotalPendingBytes, 16 * 1024 * 1024, "maxTotalPendingBytes"}, {&opts.MaxBufferedBytes, 4 * 1024 * 1024, "maxBufferedBytes"}}
	for _, d := range defaults {
		if *d.value == 0 {
			*d.value = d.fallback
		}
		if *d.value < 1 {
			return nil, fmt.Errorf("%s must be a positive integer", d.name)
		}
	}
	if opts.Window > MaxWindow {
		return nil, protocolErrorf("window must be between 1 and %d", MaxWindow)
	}
	if opts.ConnectTimeout == 0 {
		opts.ConnectTimeout = 10 * time.Second
	}
	if opts.HandshakeTimeout == 0 {
		opts.HandshakeTimeout = 15 * time.Second
	}
	if opts.LivenessTimeout == 0 {
		opts.LivenessTimeout = 90 * time.Second
	}
	if opts.ConnectTimeout < 0 || opts.HandshakeTimeout < 0 || opts.LivenessTimeout < 0 {
		return nil, errors.New("tunnel timeouts must be positive")
	}
	ctx, cancel := context.WithCancel(ctx)
	handshakeCtx, stop := context.WithTimeout(ctx, opts.HandshakeTimeout)
	defer stop()
	ws, err := dialWebSocket(handshakeCtx, remote, token)
	if err != nil {
		cancel()
		return nil, err
	}
	t := &Destination{Inspection: inspection, ctx: ctx, cancel: cancel, ws: ws, flows: map[uint32]*destinationFlow{}, recentlyClosed: map[uint32]time.Time{}, done: make(chan struct{}), opts: opts, routes: selectors.Routes, state: connectionState{value: Connecting}}
	interrupt := context.AfterFunc(handshakeCtx, func() { ws.Close() })
	defer interrupt()
	err = t.send(ClientMessage{Type: "start", Version: Version, SelectorValues: opts.Selectors, Inspection: inspection, Window: opts.Window, Name: opts.Name, SkipsInactivityReset: opts.ResetsInactivityTimeout != nil && !*opts.ResetsInactivityTimeout})
	if err != nil {
		t.fail(err)
		return nil, err
	}
	kind, data, err := ws.ReadMessage()
	if err != nil {
		t.fail(err)
		return nil, err
	}
	if kind != websocket.TextMessage {
		err = protocolErrorf("received tunnel data before READY")
	} else {
		var ready ServerMessage
		ready, err = DecodeServerMessage(data)
		if err == nil {
			if ready.Type == "error" {
				err = &DestinationSessionError{ready.Code}
			} else if ready.Type != "ready" {
				err = protocolErrorf("received %s before READY", ready.Type)
			} else if err = AssertReady(ready); err == nil {
				expected, e := ConfigHashWithInspection(selectors, inspection)
				if e != nil {
					err = e
				} else if ready.ConfigHash != expected {
					err = protocolErrorf("server acknowledged config %s but %s was negotiated", ready.ConfigHash, expected)
				} else {
					t.TunnelID = ready.TunnelID
					t.Selectors = ready.Selectors
					t.ConfigHash = ready.ConfigHash
				}
			}
		}
	}
	if err != nil {
		t.fail(err)
		return nil, err
	}
	if handshakeCtx.Err() != nil {
		t.fail(handshakeCtx.Err())
		return nil, handshakeCtx.Err()
	}
	interrupt()
	if inspection.Enabled && (opts.OnInspectionEvent != nil || opts.OnInspectionError != nil) {
		t.InspectionStream, err = StartInspectionStream(ctx, remote, t.TunnelID, token, InspectionStreamOptions{OnEvent: opts.OnInspectionEvent, OnError: opts.OnInspectionError})
		if err != nil {
			safeError(opts.OnInspectionError, err)
		}
	}
	t.state.set(Connected)
	ws.SetReadDeadline(time.Now().Add(opts.LivenessTimeout))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(opts.LivenessTimeout)) })
	ws.SetPingHandler(func(message string) error {
		ws.SetReadDeadline(time.Now().Add(opts.LivenessTimeout))
		return ws.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(10*time.Second))
	})
	go func() { <-ctx.Done(); t.Close() }()
	go t.ping()
	go t.read()
	return t, nil
}
func (t *Destination) GetConnectionState() ConnectionState { return t.state.get() }
func (t *Destination) OnConnectionStateChange(f func(ConnectionState)) func() {
	return t.state.subscribe(f)
}
func (t *Destination) Done() <-chan struct{} { return t.done }
func (t *Destination) Err() error            { t.mu.Lock(); defer t.mu.Unlock(); return t.err }
func (t *Destination) Close() error          { t.fail(nil); return nil }
func (t *Destination) fail(err error) {
	changed := false
	t.once.Do(func() {
		changed = true
		t.mu.Lock()
		t.err = err
		flows := t.flows
		t.flows = map[uint32]*destinationFlow{}
		t.pendingBytes = 0
		t.mu.Unlock()
		t.cancel()
		t.ws.Close()
		for _, flow := range flows {
			flow.cancel()
			if flow.conn != nil {
				flow.conn.Close()
			}
		}
		if t.InspectionStream != nil {
			t.InspectionStream.Close()
		}
		close(t.done)
	})
	if changed {
		t.state.set(Disconnected)
	}
}
func (t *Destination) send(m ClientMessage) error {
	data, err := EncodeClientMessage(m)
	if err != nil {
		return err
	}
	return t.write(websocket.TextMessage, data)
}
func (t *Destination) write(kind int, data []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if t.ctx.Err() != nil {
		return t.ctx.Err()
	}
	t.ws.SetWriteDeadline(time.Now().Add(t.opts.LivenessTimeout))
	return t.ws.WriteMessage(kind, data)
}
func (t *Destination) ping() {
	interval := t.opts.LivenessTimeout / 3
	if interval > 30*time.Second {
		interval = 30 * time.Second
	}
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			if err := t.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(interval)); err != nil {
				t.fail(err)
				return
			}
		}
	}
}
func (t *Destination) remove(id uint32) {
	t.mu.Lock()
	flow := t.flows[id]
	if flow != nil {
		delete(t.flows, id)
		t.pendingBytes -= flow.pendingBytes
		flow.pendingBytes = 0
		flow.pending = nil
	}
	t.recentlyClosed[id] = time.Now().Add(30 * time.Second)
	t.mu.Unlock()
	if flow != nil {
		flow.cancel()
		if flow.conn != nil {
			flow.conn.Close()
		}
	}
}
func (t *Destination) reset(id uint32, reason string) {
	if err := t.send(ClientMessage{Type: "reset", ConnID: id, Reason: reason}); err != nil {
		t.fail(err)
	}
	t.remove(id)
}
func (t *Destination) read() {
	for {
		kind, data, err := t.ws.ReadMessage()
		if err != nil {
			if t.ctx.Err() == nil {
				t.fail(err)
			}
			return
		}
		t.ws.SetReadDeadline(time.Now().Add(t.opts.LivenessTimeout))
		if kind == websocket.BinaryMessage {
			var id uint32
			var payload []byte
			id, payload, err = DecodeDataFrame(data)
			if err == nil {
				t.incoming(id, payload)
			}
		} else {
			var m ServerMessage
			m, err = DecodeServerMessage(data)
			if err == nil {
				switch m.Type {
				case "ready":
					err = protocolErrorf("received duplicate READY")
				case "error":
					err = &DestinationSessionError{m.Code}
				case "open":
					err = t.open(m)
				case "reset":
					t.remove(m.ConnID)
				case "fin":
					t.incomingFIN(m.ConnID)
				case "windowUpdate":
					t.mu.Lock()
					flow := t.flows[m.ConnID]
					if flow != nil && flow.open {
						flow.credit += int64(m.Increment)
						notify(flow.creditWake)
					}
					t.mu.Unlock()
				}
			}
		}
		if err != nil {
			t.fail(err)
			return
		}
	}
}
func (t *Destination) open(m ServerMessage) error {
	kind, err := AssertOpenAllowed(m, t.opts.Selectors)
	if err != nil {
		if err = t.send(ClientMessage{Type: "openFail", ConnID: m.ConnID, Reason: OpenFailSelectorNotAllowed}); err != nil {
			return err
		}
		t.remove(m.ConnID)
		return nil
	}
	t.mu.Lock()
	now := time.Now()
	for id, until := range t.recentlyClosed {
		if !now.Before(until) {
			delete(t.recentlyClosed, id)
		}
	}
	_, closed := t.recentlyClosed[m.ConnID]
	if t.flows[m.ConnID] != nil || closed {
		t.mu.Unlock()
		return protocolErrorf("server reused connection ID %d", m.ConnID)
	}
	if len(t.flows) >= t.opts.MaxConnections {
		t.mu.Unlock()
		err = t.send(ClientMessage{Type: "openFail", ConnID: m.ConnID, Reason: OpenFailResourceExhausted})
		t.remove(m.ConnID)
		return err
	}
	ctx, cancel := context.WithCancel(t.ctx)
	flow := &destinationFlow{ctx: ctx, cancel: cancel, credit: int64(m.Window), wake: make(chan struct{}, 1), creditWake: make(chan struct{}, 1)}
	t.flows[m.ConnID] = flow
	t.mu.Unlock()
	go t.dial(m, kind, flow)
	return nil
}
func (t *Destination) dial(m ServerMessage, kind SelectorKind, flow *destinationFlow) {
	ctx, cancel := context.WithTimeout(flow.ctx, t.opts.ConnectTimeout)
	defer cancel()
	host := m.Host
	var dnsMs *int64
	var conn net.Conn
	var result TransportResult
	var err error
	if kind == SelectorKindDomain {
		start := time.Now()
		var addresses []net.IPAddr
		addresses, err = net.DefaultResolver.LookupIPAddr(ctx, host)
		elapsed := time.Since(start).Milliseconds()
		dnsMs = &elapsed
		if err == nil {
			host = ""
			for _, ip := range addresses {
				if IsDialableResolvedAddress(ip.IP.String(), m.Port, t.routes) {
					host = ip.IP.String()
					break
				}
			}
			if host == "" {
				err = &codedError{"EBLOCKED", "all resolved addresses are blocked"}
			}
		}
	}
	if err == nil {
		start := time.Now()
		protocol := "http"
		if m.Transport.Type == TransportTLS {
			protocol = "https"
		}
		var address string
		conn, address, err = connectDestination(ctx, host, m.Host, m.Port, protocol)
		elapsed := time.Since(start).Milliseconds()
		result = TransportResult{Type: m.Transport.Type, RemoteAddress: address, ConnectMs: &elapsed, DNSMs: dnsMs}
	}
	if err == nil && m.Transport.Type == TransportTLS {
		start := time.Now()
		secure := tls.Client(conn, &tls.Config{ServerName: m.Transport.ServerName, NextProtos: m.Transport.ALPNProtocols, MinVersion: tls.VersionTLS12})
		err = secure.HandshakeContext(ctx)
		if err == nil {
			conn = secure
			elapsed := time.Since(start).Milliseconds()
			result.TLSMs = &elapsed
			result.ALPNProtocol = secure.ConnectionState().NegotiatedProtocol
		}
	}
	if err != nil {
		if conn != nil {
			conn.Close()
		}
		if flow.ctx.Err() == nil {
			code := osCode(err)
			if code == "EBLOCKED" {
				code = ""
			}
			if e := t.send(ClientMessage{Type: "openFail", ConnID: m.ConnID, Reason: ClassifyOpenFailure(err), OSCode: code}); e != nil {
				t.fail(e)
			}
			t.remove(m.ConnID)
		}
		return
	}
	t.mu.Lock()
	if t.flows[m.ConnID] != flow || t.ctx.Err() != nil {
		t.mu.Unlock()
		conn.Close()
		return
	}
	flow.conn = conn
	flow.open = true
	t.mu.Unlock()
	if err = t.send(ClientMessage{Type: "openOk", ConnID: m.ConnID, Window: t.opts.Window, TransportResult: result}); err != nil {
		t.fail(err)
		return
	}
	go t.writeFlow(m.ConnID, flow)
	t.readFlow(m.ConnID, flow)
}
func (t *Destination) findOpen(id uint32) (*destinationFlow, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	flow := t.flows[id]
	if flow != nil && flow.open {
		return flow, false
	}
	_, closed := t.recentlyClosed[id]
	return nil, !closed
}
func (t *Destination) incoming(id uint32, data []byte) {
	flow, unknown := t.findOpen(id)
	if flow == nil {
		if unknown {
			t.reset(id, ResetProtocolError)
		}
		return
	}
	t.mu.Lock()
	if t.flows[id] != flow {
		t.mu.Unlock()
		return
	}
	reason := ""
	if flow.remoteFIN {
		reason = ResetProtocolError
	} else if flow.pendingBytes+len(data) > t.opts.MaxPendingBytesPerConnection || t.pendingBytes+len(data) > t.opts.MaxTotalPendingBytes {
		reason = ResetResourceExhausted
	}
	if reason != "" {
		t.mu.Unlock()
		t.reset(id, reason)
		return
	}
	flow.pending = append(flow.pending, append([]byte(nil), data...))
	flow.pendingBytes += len(data)
	t.pendingBytes += len(data)
	notify(flow.wake)
	t.mu.Unlock()
}
func (t *Destination) incomingFIN(id uint32) {
	flow, unknown := t.findOpen(id)
	if flow == nil {
		if unknown {
			t.reset(id, ResetProtocolError)
		}
		return
	}
	t.mu.Lock()
	if flow.remoteFIN {
		t.mu.Unlock()
		t.reset(id, ResetProtocolError)
		return
	}
	flow.remoteFIN = true
	notify(flow.wake)
	t.mu.Unlock()
}
func (t *Destination) writeFlow(id uint32, flow *destinationFlow) {
	for {
		t.mu.Lock()
		if t.flows[id] != flow {
			t.mu.Unlock()
			return
		}
		if len(flow.pending) == 0 {
			if flow.remoteFIN {
				flow.remoteDrained = true
				finished := flow.localFIN
				t.mu.Unlock()
				if writer, ok := flow.conn.(interface{ CloseWrite() error }); ok {
					if err := writer.CloseWrite(); err != nil {
						t.reset(id, ResetConnectionError)
						return
					}
				}
				if finished {
					t.remove(id)
				}
				return
			}
			t.mu.Unlock()
			select {
			case <-flow.ctx.Done():
				return
			case <-flow.wake:
			}
			continue
		}
		data := flow.pending[0]
		flow.pending = flow.pending[1:]
		t.mu.Unlock()
		_, err := flow.conn.Write(data)
		if err != nil {
			if flow.ctx.Err() == nil {
				t.reset(id, ResetConnectionError)
			}
			return
		}
		t.mu.Lock()
		if t.flows[id] != flow {
			t.mu.Unlock()
			return
		}
		flow.pendingBytes -= len(data)
		t.pendingBytes -= len(data)
		flow.delivered += len(data)
		increment := 0
		if flow.delivered >= (t.opts.Window+1)/2 {
			increment = flow.delivered
			flow.delivered = 0
		}
		t.mu.Unlock()
		if increment > 0 {
			if err = t.send(ClientMessage{Type: "windowUpdate", ConnID: id, Increment: increment}); err != nil {
				t.fail(err)
				return
			}
		}
	}
}
func (t *Destination) readFlow(id uint32, flow *destinationFlow) {
	buf := make([]byte, 32*1024)
	for {
		n, readErr := flow.conn.Read(buf)
		for offset := 0; offset < n; {
			t.mu.Lock()
			if t.flows[id] != flow {
				t.mu.Unlock()
				return
			}
			credit := flow.credit
			if credit <= 0 {
				t.mu.Unlock()
				select {
				case <-flow.ctx.Done():
					return
				case <-flow.creditWake:
				}
				continue
			}
			take := n - offset
			if int64(take) > credit {
				take = int(credit)
			}
			flow.credit -= int64(take)
			t.mu.Unlock()
			if err := t.write(websocket.BinaryMessage, encodeMessage(id, buf[offset:offset+take])); err != nil {
				t.fail(err)
				return
			}
			offset += take
		}
		if readErr != nil {
			if readErr != io.EOF {
				if flow.ctx.Err() == nil {
					t.reset(id, ResetConnectionError)
				}
				return
			}
			if err := t.send(ClientMessage{Type: "fin", ConnID: id}); err != nil {
				t.fail(err)
				return
			}
			t.mu.Lock()
			flow.localFIN = true
			finished := flow.remoteDrained
			t.mu.Unlock()
			if finished {
				t.remove(id)
			}
			return
		}
	}
}
