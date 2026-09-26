// Package android provides a client for interacting with Limrun Android
// instances over the instance's control WebSocket.
//
// It speaks the same protocol as the TypeScript SDK's instance client: the
// control endpoint is the instance's status.apiUrl with the /ws path, commands
// are sent as {"type", "id", ...params, "payload": params} and answered by a
// "<type>Result" message carrying either a payload or an error. App
// installation is the exception: it is sent as an "asset" message and
// acknowledged by an "assetResult" keyed on the URL rather than a request id.
//
// Unlike iOS instances, WebSocket transport pings do not reset an Android
// instance's inactivity timer; only the application-level keepAlive message
// does. Call KeepAlive periodically, or use WithKeepAliveInterval to have the
// client send it for you.
package android

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Common errors returned by the client.
var (
	ErrNotConnected    = errors.New("websocket: not connected")
	ErrConnectionClose = errors.New("websocket: connection closed")
)

// CommandError is returned when the instance answers a command with an
// error. Code and Retriable are set when the server provides them.
type CommandError struct {
	Type      string // the result message type, e.g. "openUrlResult"
	Code      string
	Message   string
	Retriable bool
}

func (e *CommandError) Error() string {
	switch {
	case e.Message != "" && e.Code != "":
		return fmt.Sprintf("%s: %s (%s)", e.Type, e.Message, e.Code)
	case e.Message != "":
		return fmt.Sprintf("%s: %s", e.Type, e.Message)
	case e.Code != "":
		return fmt.Sprintf("%s: %s", e.Type, e.Code)
	default:
		return fmt.Sprintf("%s: server returned an error with no message or code", e.Type)
	}
}

// LaunchMode specifies how LaunchApp behaves when the app is already running.
type LaunchMode string

const (
	// LaunchModeForegroundIfRunning brings the app to the foreground if it is
	// already running, otherwise launches it.
	LaunchModeForegroundIfRunning LaunchMode = "ForegroundIfRunning"
	// LaunchModeRelaunchIfRunning stops and relaunches the app if it is
	// already running.
	LaunchModeRelaunchIfRunning LaunchMode = "RelaunchIfRunning"
)

// InstalledApp is an app installed on the instance.
type InstalledApp struct {
	BundleID    string `json:"bundleId"`
	Name        string `json:"name"`
	InstallType string `json:"installType"`
	// Icon is a base64-encoded 64px PNG, empty when the runtime could not
	// extract one.
	Icon string `json:"icon,omitempty"`
	// InstalledAt is the install time in epoch milliseconds, zero when unknown.
	InstalledAt int64 `json:"installedAt,omitempty"`
}

// ScreenshotData is the result of Screenshot.
type ScreenshotData struct {
	// DataURI is the screenshot as a data URI (e.g. "data:image/png;base64,...").
	DataURI string `json:"dataUri"`
}

// Option configures a Client.
type Option func(*Client)

// WithLogger sets a custom logger. Defaults to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) { c.logger = logger }
}

// WithDialer sets the WebSocket dialer. Defaults to websocket.DefaultDialer.
func WithDialer(dialer *websocket.Dialer) Option {
	return func(c *Client) { c.dialer = dialer }
}

// WithKeepAliveInterval makes the client send the application-level
// keepAlive message every d while connected, which keeps the instance from
// reaching its inactivity timeout. Zero (the default) disables it; call
// KeepAlive yourself instead.
func WithKeepAliveInterval(d time.Duration) Option {
	return func(c *Client) { c.keepAliveInterval = d }
}

// WithAssetTimeout sets how long InstallApp waits for the instance to
// acknowledge an install when the context has no deadline. Defaults to two
// minutes, matching the TypeScript SDK.
func WithAssetTimeout(d time.Duration) Option {
	return func(c *Client) { c.assetTimeout = d }
}

// WithRequestTimeout sets how long commands wait for a result when the
// context has no deadline. Defaults to 30 seconds, matching the TypeScript
// SDK.
func WithRequestTimeout(d time.Duration) Option {
	return func(c *Client) { c.requestTimeout = d }
}

// Client is a WebSocket client for a Limrun Android instance.
type Client struct {
	apiURL string
	token  string
	logger *slog.Logger
	dialer *websocket.Dialer

	keepAliveInterval time.Duration
	keepAliveSession  string
	assetTimeout      time.Duration
	requestTimeout    time.Duration

	ws        *websocket.Conn
	wsMu      sync.Mutex // serialises writes
	pending   sync.Map   // request id -> chan result
	assetsMu  sync.Mutex
	assets    map[string][]chan result // url -> waiters, oldest first
	requestID atomic.Uint64
	closed    atomic.Bool
	done      chan struct{}
}

// result is what a pending request receives: the raw message, or an error.
type result struct {
	msg message
	err error
}

// message is the envelope of everything the instance sends.
type message struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
	// Message carries the error text of screenshotError and assetResult.
	Message string `json:"message,omitempty"`
	// URL and Result are set on assetResult.
	URL    string `json:"url,omitempty"`
	Result string `json:"result,omitempty"`
	// DataURI is set on the legacy "screenshot" message.
	DataURI string `json:"dataUri,omitempty"`

	raw json.RawMessage
}

// NewClient connects to the control WebSocket of an Android instance.
// apiURL is the instance's status.apiUrl and token its access token.
func NewClient(apiURL, token string, opts ...Option) (*Client, error) {
	c := &Client{
		apiURL:           apiURL,
		token:            token,
		logger:           slog.Default(),
		dialer:           websocket.DefaultDialer,
		keepAliveSession: newSessionID(),
		assetTimeout:     2 * time.Minute,
		requestTimeout:   30 * time.Second,
		assets:           map[string][]chan result{},
		done:             make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}

	wsURL, err := controlURL(apiURL, token)
	if err != nil {
		return nil, err
	}
	ws, _, err := c.dialer.Dial(wsURL, http.Header{})
	if err != nil {
		return nil, fmt.Errorf("websocket dial: %w", err)
	}
	c.ws = ws

	go c.readLoop()
	go c.pingLoop()
	if c.keepAliveInterval > 0 {
		go c.keepAliveLoop()
	}
	return c, nil
}

// controlURL converts an http(s) API URL into the ws(s) control endpoint,
// including the auth token query parameter.
func controlURL(apiURL, token string) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", fmt.Errorf("invalid API URL: %w", err)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("invalid API URL scheme %q", u.Scheme)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/ws"
	u.RawPath = ""
	u.Fragment = ""
	q := url.Values{}
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("go-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// Close closes the connection and fails every pending request.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	close(c.done)

	c.wsMu.Lock()
	err := c.ws.Close()
	c.wsMu.Unlock()

	c.failAll(ErrConnectionClose)
	return err
}

func (c *Client) failAll(err error) {
	c.pending.Range(func(key, _ any) bool {
		// LoadAndDelete so a result arriving concurrently from dispatch and
		// this failure never both send on the same channel.
		if ch, ok := c.pending.LoadAndDelete(key); ok {
			ch.(chan result) <- result{err: err}
		}
		return true
	})
	c.assetsMu.Lock()
	for u, waiters := range c.assets {
		for _, ch := range waiters {
			ch <- result{err: err}
		}
		delete(c.assets, u)
	}
	c.assetsMu.Unlock()
}

func (c *Client) readLoop() {
	defer c.failAll(ErrConnectionClose)
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			if !c.closed.Load() {
				c.logger.Error("websocket read error", "error", err)
			}
			return
		}
		var msg message
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Error("failed to parse message", "error", err)
			continue
		}
		msg.raw = data
		c.dispatch(msg)
	}
}

func (c *Client) dispatch(msg message) {
	switch msg.Type {
	case "assetResult":
		c.assetsMu.Lock()
		waiters := c.assets[msg.URL]
		if len(waiters) == 0 {
			c.assetsMu.Unlock()
			c.logger.Warn("assetResult for unknown or already handled url", "url", msg.URL)
			return
		}
		ch := waiters[0]
		if len(waiters) == 1 {
			delete(c.assets, msg.URL)
		} else {
			c.assets[msg.URL] = waiters[1:]
		}
		c.assetsMu.Unlock()
		ch <- result{msg: msg}
	case "appExit":
		// Delivered for launches and watches that registered an execId; this
		// client does not register any yet.
		c.logger.Debug("ignoring appExit", "message", string(msg.raw))
	default:
		if msg.ID == "" {
			c.logger.Warn("unexpected message without id", "type", msg.Type)
			return
		}
		if ch, ok := c.pending.LoadAndDelete(msg.ID); ok {
			ch.(chan result) <- result{msg: msg}
		} else {
			c.logger.Debug("result for unknown or already settled request", "type", msg.Type, "id", msg.ID)
		}
	}
}

func (c *Client) pingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.wsMu.Lock()
			_ = c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
			c.wsMu.Unlock()
		}
	}
}

func (c *Client) keepAliveLoop() {
	ticker := time.NewTicker(c.keepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if err := c.KeepAlive(); err != nil && !c.closed.Load() {
				c.logger.Warn("keepAlive failed", "error", err)
			}
		}
	}
}

func (c *Client) write(v any) error {
	if c.closed.Load() {
		return ErrNotConnected
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	c.wsMu.Lock()
	defer c.wsMu.Unlock()
	return c.ws.WriteMessage(websocket.TextMessage, data)
}

// withDefaultTimeout applies d when ctx has no deadline of its own.
func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok || d <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

// command sends a request and decodes the payload of its result into out
// (which may be nil). Params are sent both flat and under "payload", exactly
// like the TypeScript SDK, so the server can read either.
func (c *Client) command(ctx context.Context, typ string, params map[string]any, out any) (message, error) {
	ctx, cancel := withDefaultTimeout(ctx, c.requestTimeout)
	defer cancel()

	id := fmt.Sprintf("go-client-%d-%d", time.Now().UnixMilli(), c.requestID.Add(1))
	req := map[string]any{"type": typ, "id": id}
	if len(params) > 0 {
		for k, v := range params {
			req[k] = v
		}
		req["payload"] = params
	}

	ch := make(chan result, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	c.logger.Debug("sending request", "type", typ, "id", id)
	if err := c.write(req); err != nil {
		return message{}, fmt.Errorf("send %s: %w", typ, err)
	}

	select {
	case <-ctx.Done():
		return message{}, fmt.Errorf("%s: %w", typ, ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return message{}, r.err
		}
		if err := resultError(r.msg); err != nil {
			return r.msg, err
		}
		if out != nil && len(r.msg.Payload) > 0 && string(r.msg.Payload) != "null" {
			if err := json.Unmarshal(r.msg.Payload, out); err != nil {
				return r.msg, fmt.Errorf("decode %s payload: %w", r.msg.Type, err)
			}
		}
		return r.msg, nil
	}
}

// resultError extracts an error from a result message. The server reports
// errors three ways: an {code, message, retriable} object, a plain string on
// "flat" results, and a top-level message on screenshotError.
func resultError(msg message) error {
	if msg.Type == "screenshotError" {
		return &CommandError{Type: msg.Type, Message: msg.Message}
	}
	if len(msg.Error) == 0 || string(msg.Error) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(msg.Error, &s); err == nil {
		if s == "" {
			return nil
		}
		return &CommandError{Type: msg.Type, Message: s}
	}
	var obj struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		Retriable bool   `json:"retriable"`
	}
	if err := json.Unmarshal(msg.Error, &obj); err != nil {
		return &CommandError{Type: msg.Type, Message: string(msg.Error)}
	}
	return &CommandError{Type: msg.Type, Code: obj.Code, Message: obj.Message, Retriable: obj.Retriable}
}

// ============================================================================
// Client Methods
// ============================================================================

// KeepAlive sends the application-level keepAlive message, which resets the
// instance's inactivity timer. It is fire-and-forget. Transport pings alone do
// not keep an Android instance alive.
func (c *Client) KeepAlive() error {
	return c.write(struct {
		Type      string `json:"type"`
		SessionID string `json:"sessionId"`
	}{Type: "keepAlive", SessionID: c.keepAliveSession})
}

// InstallApp installs an APK from a URL the instance can download. It returns
// once the instance acknowledges the install.
//
// Installs are acknowledged by URL, not by request id, so concurrent installs
// of the same URL are resolved in the order they were sent.
func (c *Client) InstallApp(ctx context.Context, apkURL string) error {
	ctx, cancel := withDefaultTimeout(ctx, c.assetTimeout)
	defer cancel()

	ch := make(chan result, 1)
	c.assetsMu.Lock()
	c.assets[apkURL] = append(c.assets[apkURL], ch)
	c.assetsMu.Unlock()

	if err := c.write(map[string]string{"type": "asset", "url": apkURL}); err != nil {
		c.dropAssetWaiter(apkURL, ch)
		return fmt.Errorf("send asset: %w", err)
	}

	select {
	case <-ctx.Done():
		c.dropAssetWaiter(apkURL, ch)
		return fmt.Errorf("install %s: %w", apkURL, ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if r.msg.Result == "success" {
			return nil
		}
		msg := r.msg.Message
		if msg == "" {
			msg = fmt.Sprintf("asset processing failed: %s", r.msg.raw)
		}
		return &CommandError{Type: "assetResult", Message: msg}
	}
}

func (c *Client) dropAssetWaiter(apkURL string, ch chan result) {
	c.assetsMu.Lock()
	defer c.assetsMu.Unlock()
	waiters := c.assets[apkURL]
	for i, w := range waiters {
		if w == ch {
			waiters = append(waiters[:i:i], waiters[i+1:]...)
			break
		}
	}
	if len(waiters) == 0 {
		delete(c.assets, apkURL)
	} else {
		c.assets[apkURL] = waiters
	}
}

// OpenURL opens a URL (web or deep link) on the instance and returns the URL
// the instance reported opening.
func (c *Client) OpenURL(ctx context.Context, u string) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	if _, err := c.command(ctx, "openUrl", map[string]any{"url": u}, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return u, nil
	}
	return out.URL, nil
}

// LaunchApp launches an installed app by package name. An empty mode leaves
// the choice to the instance.
func (c *Client) LaunchApp(ctx context.Context, packageName string, mode LaunchMode) error {
	params := map[string]any{"packageName": packageName}
	if mode != "" {
		params["mode"] = mode
	}
	ctx, cancel := withDefaultTimeout(ctx, 60*time.Second) // launches can be slow; TS SDK uses 60s
	defer cancel()
	_, err := c.command(ctx, "launchApp", params, nil)
	return err
}

// TerminateApp stops a running app by package name.
func (c *Client) TerminateApp(ctx context.Context, packageName string) error {
	_, err := c.command(ctx, "terminateApp", map[string]any{"packageName": packageName}, nil)
	return err
}

// ListApps returns the apps installed on the instance.
func (c *Client) ListApps(ctx context.Context) ([]InstalledApp, error) {
	var out struct {
		Apps []InstalledApp `json:"apps"`
	}
	if _, err := c.command(ctx, "listApps", nil, &out); err != nil {
		return nil, err
	}
	return out.Apps, nil
}

// Screenshot captures the current screen.
func (c *Client) Screenshot(ctx context.Context) (*ScreenshotData, error) {
	var out ScreenshotData
	msg, err := c.command(ctx, "screenshot", nil, &out)
	if err != nil {
		return nil, err
	}
	if out.DataURI == "" {
		// Older runtimes answer with a top-level "screenshot" message.
		out.DataURI = msg.DataURI
	}
	if out.DataURI == "" {
		return nil, errors.New("screenshotResult without payload.dataUri")
	}
	return &out, nil
}
