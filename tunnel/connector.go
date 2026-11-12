package tunnel

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/limrun-inc/go-sdk/internal"
)

type ConnectorOptions struct {
	// WebSocketDialer controls WebSocket connections. Nil uses the SDK default.
	// A custom dialer controls its own proxy settings and must not be modified during use.
	WebSocketDialer                          *websocket.Dialer
	Token, BaseURL, OrganizationID, TunnelID string
	Inspection                               *InspectionConfig
	Replace                                  bool
	ResetsInactivityTimeout                  *bool
	Hostname, ClientVersion                  string
	OnEvent                                  func(ConnectorEvent)
}
type ConnectorHolder struct {
	Hostname string `json:"hostname"`
	Since    string `json:"since,omitempty"`
}
type ConnectorEvent struct {
	Type, SessionID, Name, ExpiresAt, InstanceID, TunnelID, Code, Message, Reason string
	Holder                                                                        *ConnectorHolder
	DelayMs                                                                       int64
}
type Connector struct {
	ctx                                                context.Context
	cancel                                             context.CancelFunc
	once                                               sync.Once
	done                                               chan struct{}
	mu                                                 sync.Mutex
	writeMu                                            sync.Mutex
	err                                                error
	ws                                                 *websocket.Conn
	opts                                               ConnectorOptions
	instances                                          map[string]*connectorAttachment
	sessionID, name, announcedSession, announcedHolder string
	replace                                            bool
	leaseExpires, lastWarning                          time.Time
	backoff                                            time.Duration
	eventMu                                            sync.Mutex
	events                                             []ConnectorEvent
	eventWake                                          chan struct{}
	eventsClosed                                       bool
}
type connectorAttachment struct {
	ctx          context.Context
	cancel       context.CancelFunc
	url, token   string
	expires      time.Time
	selectors    []string
	tunnel       *Destination
	lastID       string
	mayReclaim   bool
	failingSince time.Time
	backoff      time.Duration
}
type connectorMessage struct {
	kind, sessionID, name, tokenExpiresAt, instanceID, url, token, reason, code, message string
	leaseSeconds, expiresInSeconds                                                       int64
	selectors                                                                            []string
	holder                                                                               *ConnectorHolder
}

func RunTunnel(ctx context.Context, opts ConnectorOptions) (*Connector, error) {
	inspection := InspectionConfig{Enabled: true}
	if opts.Inspection != nil {
		inspection = *opts.Inspection
	}
	normalized, err := NormalizeInspectionConfig(inspection)
	if err != nil {
		return nil, err
	}
	opts.Inspection = &normalized
	endpoint, err := deriveEndpoint(opts.BaseURL, "v1/organizations/"+url.PathEscape(opts.OrganizationID)+"/tunnels/"+url.PathEscape(opts.TunnelID)+"/connect", true)
	if err != nil {
		return nil, err
	}
	if opts.Hostname == "" {
		opts.Hostname, _ = os.Hostname()
	}
	if opts.ClientVersion == "" {
		opts.ClientVersion = internal.PackageVersion
	}
	ctx, cancel := context.WithCancel(ctx)
	c := &Connector{ctx: ctx, cancel: cancel, done: make(chan struct{}), opts: opts, instances: map[string]*connectorAttachment{}, replace: opts.Replace, backoff: 500 * time.Millisecond, eventWake: make(chan struct{}, 1)}
	go c.dispatchEvents()
	go c.control(endpoint)
	go func() { <-ctx.Done(); c.shutdown(nil) }()
	return c, nil
}
func (c *Connector) Done() <-chan struct{} { return c.done }
func (c *Connector) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.err
	}
}
func (c *Connector) Close() error { c.shutdown(nil); return c.Wait(context.Background()) }
func (c *Connector) emit(event ConnectorEvent) {
	c.eventMu.Lock()
	if !c.eventsClosed {
		c.events = append(c.events, event)
		notify(c.eventWake)
	}
	c.eventMu.Unlock()
}
func (c *Connector) dispatchEvents() {
	for {
		c.eventMu.Lock()
		if len(c.events) > 0 {
			event := c.events[0]
			c.events = c.events[1:]
			c.eventMu.Unlock()
			if c.opts.OnEvent != nil {
				func() { defer func() { _ = recover() }(); c.opts.OnEvent(event) }()
			}
			continue
		}
		closed := c.eventsClosed
		c.eventMu.Unlock()
		if closed {
			return
		}
		<-c.eventWake
	}
}
func (c *Connector) shutdown(cause error) {
	c.once.Do(func() {
		c.mu.Lock()
		if cause != nil {
			name := c.name
			if name == "" {
				name = c.opts.TunnelID
			}
			c.err = fmt.Errorf("tunnel %s: %w", name, cause)
		}
		ws := c.ws
		c.cancel()
		tunnels := []*Destination{}
		for _, a := range c.instances {
			a.cancel()
			if a.tunnel != nil {
				tunnels = append(tunnels, a.tunnel)
				a.tunnel = nil
			}
		}
		c.instances = map[string]*connectorAttachment{}
		c.mu.Unlock()
		for _, tunnel := range tunnels {
			tunnel.Close()
		}
		go func() {
			if ws != nil {
				if cause == nil {
					c.writeMu.Lock()
					ws.SetWriteDeadline(time.Now().Add(time.Second))
					_ = ws.WriteJSON(map[string]any{"type": "bye"})
					c.writeMu.Unlock()
					_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(1000, "bye"), time.Now().Add(time.Second))
					time.Sleep(time.Second)
				}
				ws.Close()
			}
			close(c.done)
			c.eventMu.Lock()
			c.eventsClosed = true
			c.eventMu.Unlock()
			notify(c.eventWake)
		}()
	})
}
func (c *Connector) send(message any) {
	c.mu.Lock()
	ws := c.ws
	c.mu.Unlock()
	if ws == nil {
		return
	}
	c.writeMu.Lock()
	ws.SetWriteDeadline(time.Now().Add(15 * time.Second))
	err := ws.WriteJSON(message)
	c.writeMu.Unlock()
	if err != nil {
		ws.Close()
	}
}
func jitter(delay time.Duration) time.Duration {
	return time.Duration(float64(delay) * (.75 + rand.Float64()*.5))
}
func (c *Connector) control(endpoint string) {
	for c.ctx.Err() == nil {
		handshakeCtx, cancel := context.WithTimeout(c.ctx, 15*time.Second)
		ws, err := dialWebSocket(handshakeCtx, endpoint, c.opts.Token, c.opts.WebSocketDialer)
		cancel()
		prompt := false
		reason := ""
		if err == nil {
			c.mu.Lock()
			if c.ctx.Err() != nil {
				c.mu.Unlock()
				ws.Close()
				return
			}
			c.ws = ws
			c.announcedSession = ""
			attached := []string{}
			for id, a := range c.instances {
				if a.tunnel != nil {
					attached = append(attached, id)
				}
			}
			hello := map[string]any{"type": "hello", "version": 2, "replace": c.replace, "attached": attached, "client": map[string]any{"hostname": c.opts.Hostname, "version": c.opts.ClientVersion}}
			if c.sessionID != "" {
				hello["sessionId"] = c.sessionID
			}
			c.mu.Unlock()
			c.send(hello)
			readCtx, stop := context.WithCancel(c.ctx)
			ws.SetReadDeadline(time.Now().Add(45 * time.Second))
			ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(45 * time.Second)) })
			ws.SetPingHandler(func(s string) error {
				ws.SetReadDeadline(time.Now().Add(45 * time.Second))
				return ws.WriteControl(websocket.PongMessage, []byte(s), time.Now().Add(10*time.Second))
			})
			go func() {
				ticker := time.NewTicker(15 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-readCtx.Done():
						return
					case <-ticker.C:
						if e := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); e != nil {
							ws.Close()
							return
						}
					}
				}
			}()
			for c.ctx.Err() == nil {
				kind, data, e := ws.ReadMessage()
				if e != nil {
					err = e
					break
				}
				ws.SetReadDeadline(time.Now().Add(45 * time.Second))
				if kind != websocket.TextMessage {
					err = protocolErrorf("unexpected binary control frame")
					reason = "invalid control message: " + err.Error()
					break
				}
				message, e := decodeConnectorMessage(data)
				if e != nil {
					err = e
					reason = "invalid control message: " + e.Error()
					break
				}
				if message.kind != "" {
					c.handle(message)
				}
			}
			stop()
			ws.Close()
			c.mu.Lock()
			if c.ws == ws {
				c.ws = nil
			}
			c.mu.Unlock()
			if c.ctx.Err() != nil {
				return
			}
			if reason == "" {
				var closeErr *websocket.CloseError
				if errors.As(err, &closeErr) {
					detail := ""
					if closeErr.Text != "" {
						detail = ": " + closeErr.Text
					}
					switch closeErr.Code {
					case 1000:
						c.shutdown(errors.New("the server ended the session" + detail))
						return
					case 1012, 1013:
						reason = fmt.Sprintf("the server asked to reconnect (%d)", closeErr.Code)
						prompt = true
					case 4400:
						c.shutdown(errors.New("the server rejected the connector" + detail))
						return
					case 4404:
						c.shutdown(errors.New("the tunnel was deleted"))
						return
					case 4409:
						c.shutdown(errors.New("the tunnel was revoked" + detail))
						return
					default:
						reason = fmt.Sprintf("the control connection closed (%d%s)", closeErr.Code, detail)
					}
				} else if err != nil {
					reason = err.Error()
				}
			}
		} else {
			status := UpgradeStatus(err.Error())
			if status >= 400 && status < 500 && status != 429 {
				c.shutdown(fmt.Errorf("the server refused the connection (%v)", err))
				return
			}
			reason = err.Error()
		}
		if c.ctx.Err() != nil {
			return
		}
		c.mu.Lock()
		delay := c.backoff
		if prompt {
			delay = 500 * time.Millisecond
		} else {
			c.backoff *= 2
			if c.backoff > 10*time.Second {
				c.backoff = 10 * time.Second
			}
		}
		c.mu.Unlock()
		delay = jitter(delay)
		c.emit(ConnectorEvent{Type: "reconnecting", DelayMs: delay.Milliseconds(), Reason: reason})
		if !wait(c.ctx, delay) {
			return
		}
	}
}
func (c *Connector) handle(m connectorMessage) {
	if c.ctx.Err() != nil {
		return
	}
	switch m.kind {
	case "active":
		c.mu.Lock()
		c.replace = false
		c.leaseExpires = time.Now().Add(time.Duration(m.leaseSeconds) * time.Second)
		c.backoff = 500 * time.Millisecond
		c.sessionID = m.sessionID
		c.name = m.name
		c.announcedHolder = ""
		announce := c.announcedSession != m.sessionID
		c.announcedSession = m.sessionID
		warn := false
		if expires, e := time.Parse(time.RFC3339, m.tokenExpiresAt); e == nil && time.Until(expires) < 14*24*time.Hour && time.Since(c.lastWarning) >= 24*time.Hour {
			warn = true
			c.lastWarning = time.Now()
		}
		c.mu.Unlock()
		if announce {
			c.emit(ConnectorEvent{Type: "active", SessionID: m.sessionID, Name: m.name})
		}
		if warn {
			c.emit(ConnectorEvent{Type: "tokenExpiring", ExpiresAt: m.tokenExpiresAt})
		}
	case "standby":
		c.mu.Lock()
		c.leaseExpires = time.Time{}
		c.backoff = 500 * time.Millisecond
		c.sessionID = ""
		c.announcedSession = ""
		key := m.holder.Hostname + "\x00" + m.holder.Since
		announce := c.announcedHolder != key
		c.announcedHolder = key
		c.mu.Unlock()
		if announce {
			c.emit(ConnectorEvent{Type: "standby", Holder: m.holder})
		}
	case "token":
		c.mu.Lock()
		if a := c.instances[m.instanceID]; a != nil {
			a.token = m.token
			a.expires = time.Now().Add(time.Duration(m.expiresInSeconds) * time.Second)
		}
		c.mu.Unlock()
	case "attach":
		c.mu.Lock()
		existing := c.instances[m.instanceID]
		if existing != nil {
			existing.url = m.url
			existing.token = m.token
			existing.expires = time.Now().Add(time.Duration(m.expiresInSeconds) * time.Second)
			tunnel := existing.tunnel
			c.mu.Unlock()
			if tunnel != nil {
				c.send(map[string]any{"type": "attached", "instanceId": m.instanceID, "tunnelId": tunnel.TunnelID})
			}
			return
		}
		c.mu.Unlock()
		selectors, err := ValidateSelectorValues(m.selectors)
		if err != nil {
			c.givenUp(m.instanceID, "invalid_selectors", err.Error())
			return
		}
		ctx, cancel := context.WithCancel(c.ctx)
		a := &connectorAttachment{ctx: ctx, cancel: cancel, url: m.url, token: m.token, expires: time.Now().Add(time.Duration(m.expiresInSeconds) * time.Second), selectors: selectors, mayReclaim: true, failingSince: time.Now(), backoff: time.Second}
		c.mu.Lock()
		if c.ctx.Err() != nil {
			c.mu.Unlock()
			cancel()
			return
		}
		c.instances[m.instanceID] = a
		c.mu.Unlock()
		go c.attach(m.instanceID, a)
	case "detach":
		c.mu.Lock()
		a := c.instances[m.instanceID]
		c.mu.Unlock()
		if a != nil {
			c.forget(m.instanceID, a)
			c.emit(ConnectorEvent{Type: "detached", InstanceID: m.instanceID, Reason: m.reason})
		}
	case "notice":
		c.emit(ConnectorEvent{Type: "notice", InstanceID: m.instanceID, Code: m.code, Message: m.message})
	case "revoked":
		message := "the tunnel was revoked (" + m.reason + ")"
		switch m.reason {
		case "replaced":
			message = "another connector replaced this one"
		case "deleted":
			message = "the tunnel was deleted"
		case "credential":
			message = "its tunnel token was rotated, revoked or has expired"
		}
		c.shutdown(errors.New(message))
	}
}
func (c *Connector) forget(id string, a *connectorAttachment) {
	c.mu.Lock()
	if c.instances[id] == a {
		delete(c.instances, id)
	}
	tunnel := a.tunnel
	a.tunnel = nil
	c.mu.Unlock()
	a.cancel()
	if tunnel != nil {
		tunnel.Close()
	}
}
func (c *Connector) givenUp(id, code, message string) {
	c.send(map[string]any{"type": "attachFailed", "instanceId": id, "code": code, "message": message, "terminal": true})
	c.emit(ConnectorEvent{Type: "attachFailed", InstanceID: id, Code: code, Message: message})
}
func (c *Connector) giveUp(id string, a *connectorAttachment, code, message string) {
	if a.ctx.Err() != nil {
		return
	}
	c.forget(id, a)
	c.givenUp(id, code, message)
}
func (c *Connector) retry(id string, a *connectorAttachment, code, message string) bool {
	if a.ctx.Err() != nil {
		return false
	}
	c.mu.Lock()
	if time.Since(a.failingSince) >= 5*time.Minute {
		c.mu.Unlock()
		c.giveUp(id, a, "attach_timeout", "could not attach within 5 minutes: "+message)
		return false
	}
	delay := jitter(a.backoff)
	maximum := 30 * time.Second
	if a.lastID == "" {
		maximum = 2 * time.Second
	}
	a.backoff *= 2
	if a.backoff > maximum {
		a.backoff = maximum
	}
	c.mu.Unlock()
	if code != "" {
		c.send(map[string]any{"type": "attachFailed", "instanceId": id, "code": code, "message": message, "terminal": false})
	}
	c.emit(ConnectorEvent{Type: "reconnecting", InstanceID: id, DelayMs: delay.Milliseconds(), Reason: message})
	return wait(a.ctx, delay)
}
func (c *Connector) attach(id string, a *connectorAttachment) {
	for a.ctx.Err() == nil {
		c.mu.Lock()
		base, token, expires, lastID, name := a.url, a.token, a.expires, a.lastID, c.name
		holder := time.Now().Before(c.leaseExpires)
		c.mu.Unlock()
		if !holder {
			if !c.retry(id, a, "not_holder", "this connector does not hold the tunnel") {
				return
			}
			continue
		}
		if time.Until(expires) <= 5*time.Second {
			if !c.retry(id, a, "token_expired", "waiting for a fresh attach token") {
				return
			}
			continue
		}
		if lastID != "" {
			status, err := GetDestinationStatus(a.ctx, base, token)
			if err == nil && status.Active != nil && status.Active.Name == name && status.Active.State != "stopping" && status.Active.TunnelID != lastID {
				c.giveUp(id, a, "taken_over", "another connector for this tunnel took the instance over")
				return
			}
		}
		if a.ctx.Err() != nil {
			return
		}
		remote, err := DestinationURL(base)
		var tunnel *Destination
		if err == nil {
			tunnel, err = StartDestinationTCP(a.ctx, remote, token, DestinationOptions{WebSocketDialer: c.opts.WebSocketDialer, Selectors: a.selectors, Inspection: c.opts.Inspection, ResetsInactivityTimeout: c.opts.ResetsInactivityTimeout, Name: name})
		}
		if err != nil {
			if a.ctx.Err() != nil {
				return
			}
			if UpgradeStatus(err.Error()) == 404 {
				c.giveUp(id, a, "instance_gone", "the instance is gone or terminating")
				return
			}
			var session *DestinationSessionError
			if !errors.As(err, &session) {
				if !c.retry(id, a, "connect_failed", err.Error()) {
					return
				}
				continue
			}
			if session.Code == "already_active" {
				status, e := GetDestinationStatus(a.ctx, base, token)
				if a.ctx.Err() != nil {
					return
				}
				if e != nil {
					if !c.retry(id, a, "already_active", e.Error()) {
						return
					}
					continue
				}
				active := status.Active
				if active == nil || active.State == "stopping" {
					if !c.retry(id, a, "already_active", "the previous tunnel is still stopping") {
						return
					}
					continue
				}
				if active.Name != name {
					c.giveUp(id, a, "instance_busy", "the instance already has another tunnel")
					return
				}
				if active.TunnelID == lastID {
					_ = StopDestination(a.ctx, base, token, active.TunnelID)
					if !c.retry(id, a, "already_active", "stopping the dropped tunnel") {
						return
					}
					continue
				}
				c.mu.Lock()
				canReclaim := a.mayReclaim && time.Now().Before(c.leaseExpires)
				if canReclaim {
					a.mayReclaim = false
				}
				c.mu.Unlock()
				if canReclaim {
					_ = StopDestination(a.ctx, base, token, active.TunnelID)
					continue
				}
				c.giveUp(id, a, "taken_over", "another connector for this tunnel took the instance over")
				return
			}
			if session.Code == "unavailable" || session.Code == "internal" {
				if !c.retry(id, a, session.Code, "the instance could not start the tunnel: "+session.Code) {
					return
				}
				continue
			}
			c.giveUp(id, a, session.Code, "the instance rejected the tunnel: "+session.Code)
			return
		}
		c.mu.Lock()
		if a.ctx.Err() != nil {
			c.mu.Unlock()
			tunnel.Close()
			return
		}
		a.tunnel = tunnel
		a.lastID = tunnel.TunnelID
		a.mayReclaim = false
		a.backoff = time.Second
		c.mu.Unlock()
		if tunnel.GetConnectionState() != Disconnected {
			c.send(map[string]any{"type": "attached", "instanceId": id, "tunnelId": tunnel.TunnelID})
			c.emit(ConnectorEvent{Type: "attached", InstanceID: id, TunnelID: tunnel.TunnelID})
		}
		select {
		case <-a.ctx.Done():
			return
		case <-tunnel.Done():
		}
		c.mu.Lock()
		if a.tunnel == tunnel {
			a.tunnel = nil
		}
		a.failingSince = time.Now()
		c.mu.Unlock()
		if !c.retry(id, a, "", "the instance tunnel disconnected") {
			return
		}
	}
}
func decodeConnectorMessage(data []byte) (m connectorMessage, err error) {
	r, err := readRecord(data)
	if err != nil {
		return m, err
	}
	m.kind, err = readString(r, "type")
	if err != nil {
		return m, err
	}
	positive := func(key string) (int64, error) {
		n, e := readInteger(r, key)
		if e == nil && n < 1 {
			e = protocolErrorf("%s must be a positive integer", key)
		}
		return n, e
	}
	switch m.kind {
	case "active":
		if m.sessionID, err = requiredString(r, "sessionId"); err != nil {
			return m, err
		}
		if m.name, err = requiredString(r, "name"); err != nil {
			return m, err
		}
		if m.leaseSeconds, err = positive("leaseSeconds"); err != nil {
			return m, err
		}
		m.tokenExpiresAt, err = readOptionalString(r, "tokenExpiresAt")
	case "standby":
		holder, e := readRecord(r["holder"])
		if e != nil {
			return m, e
		}
		m.holder = &ConnectorHolder{}
		if m.holder.Hostname, err = readString(holder, "hostname"); err != nil {
			return m, err
		}
		m.holder.Since, err = readOptionalString(holder, "since")
	case "attach", "token":
		if m.instanceID, err = requiredString(r, "instanceId"); err != nil {
			return m, err
		}
		if m.token, err = requiredString(r, "token"); err != nil {
			return m, err
		}
		if m.expiresInSeconds, err = positive("expiresInSeconds"); err != nil {
			return m, err
		}
		if m.kind == "attach" {
			if m.url, err = requiredString(r, "url"); err != nil {
				return m, err
			}
			m.selectors, err = readStringArray(r, "selectors", "selector")
		}
	case "detach":
		if m.instanceID, err = requiredString(r, "instanceId"); err != nil {
			return m, err
		}
		m.reason, err = readString(r, "reason")
	case "notice":
		if m.code, err = readString(r, "code"); err != nil {
			return m, err
		}
		if m.message, err = readString(r, "message"); err != nil {
			return m, err
		}
		m.instanceID, err = readOptionalString(r, "instanceId")
	case "revoked":
		m.reason, err = readString(r, "reason")
	default:
		m.kind = ""
	}
	return m, err
}
