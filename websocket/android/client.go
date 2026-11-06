package android

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/limrun-inc/go-sdk/internal/control"
	"github.com/limrun-inc/go-sdk/internal/direct"
	"github.com/limrun-inc/go-sdk/internal/proxy"
)

type ConnectionState = control.State
type Option func(*Client)

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http.HTTPClient = h } }
func WithLogger(l *slog.Logger) Option     { return func(c *Client) { c.logger = l } }
func WithADBURL(url string) Option         { return func(c *Client) { c.adbURL = url } }
func WithADBPath(path string) Option       { return func(c *Client) { c.adbPath = path } }
func WithReconnect(attempts int, delay, maxDelay time.Duration) Option {
	return func(c *Client) {
		c.connectionOptions.MaxReconnectAttempts = attempts
		c.connectionOptions.ReconnectDelay = delay
		c.connectionOptions.MaxReconnectDelay = maxDelay
	}
}

type assetPending struct{ done chan error }
type Client struct {
	transport                  *control.Client
	http                       direct.Client
	adbURL, adbPath, sessionID string
	logger                     *slog.Logger
	connectionOptions          control.Options
	callbacks                  sync.Map
	assetsMu                   sync.Mutex
	assets                     map[string][]*assetPending
}

func NewClient(apiURL, token string, opts ...Option) (*Client, error) {
	c := &Client{http: direct.Client{URL: strings.TrimRight(apiURL, "/"), Token: token, HTTPClient: proxy.Client()}, adbPath: "adb", sessionID: control.ID(), logger: slog.Default(), assets: make(map[string][]*assetPending), connectionOptions: control.Options{MaxReconnectAttempts: 6}}
	for _, opt := range opts {
		opt(c)
	}
	endpoint, err := control.URL(apiURL, token, "ws")
	if err != nil {
		return nil, err
	}
	c.connectionOptions.OnMessage = c.handleMessage
	c.connectionOptions.OnDisconnect = c.failAssets
	c.connectionOptions.OnPermanentDisconnect = func() { c.callbacks.Range(func(k, v any) bool { c.callbacks.Delete(k); return true }) }
	c.transport, err = control.New(context.Background(), endpoint, c.connectionOptions)
	if err != nil {
		return nil, err
	}
	return c, nil
}
func (c *Client) Close() error {
	c.callbacks.Range(func(k, v any) bool { c.callbacks.Delete(k); return true })
	return c.transport.Close()
}
func (c *Client) Disconnect() error                   { return c.Close() }
func (c *Client) GetConnectionState() ConnectionState { return c.transport.State() }
func (c *Client) OnConnectionStateChange(fn func(ConnectionState)) func() {
	return c.transport.OnStateChange(fn)
}
func (c *Client) KeepAlive() error {
	return c.transport.Send(map[string]any{"type": "keepAlive", "sessionId": c.sessionID})
}
func (c *Client) call(ctx context.Context, kind string, params any, out any, timeout time.Duration) error {
	fields := map[string]any{}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, &fields); err != nil {
			return err
		}
	}
	if len(fields) > 0 {
		copy := make(map[string]any, len(fields))
		for k, v := range fields {
			copy[k] = v
		}
		fields["payload"] = copy
	}
	raw, err := c.transport.Call(ctx, kind, fields, timeout)
	if err != nil {
		return err
	}
	var envelope struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
		Message string          `json:"message"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if envelope.Type == "screenshotError" {
		return errors.New(envelope.Message)
	}
	if len(envelope.Payload) > 0 && string(envelope.Payload) != "null" {
		raw = envelope.Payload
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}
func (c *Client) handleMessage(data json.RawMessage) bool {
	var msg struct {
		Type   string `json:"type"`
		ExecID string `json:"execId"`
		AppExitInfo
		Logs    []string `json:"logs"`
		URL     string   `json:"url"`
		Result  string   `json:"result"`
		Message string   `json:"message"`
	}
	if json.Unmarshal(data, &msg) != nil {
		return false
	}
	switch msg.Type {
	case "appExit":
		if msg.ExecID == "" || msg.PackageName == "" || msg.Reason == "" {
			return true
		}
		if fn, ok := c.callbacks.LoadAndDelete(msg.ExecID); ok {
			go func() {
				defer func() {
					if p := recover(); p != nil {
						c.logger.Error("app callback panicked", "panic", p)
					}
				}()
				fn.(LaunchAppExitCallback)(msg.Logs, msg.AppExitInfo)
			}()
		}
		return true
	case "assetResult":
		c.assetsMu.Lock()
		queue := c.assets[msg.URL]
		if len(queue) > 0 {
			pending := queue[0]
			if len(queue) == 1 {
				delete(c.assets, msg.URL)
			} else {
				c.assets[msg.URL] = queue[1:]
			}
			var err error
			if msg.Result != "success" {
				err = errors.New(msg.Message)
				if msg.Message == "" {
					err = fmt.Errorf("asset processing failed: %s", data)
				}
			}
			pending.done <- err
		}
		c.assetsMu.Unlock()
		return true
	}
	return false
}
func (c *Client) failAssets(err error) {
	c.assetsMu.Lock()
	defer c.assetsMu.Unlock()
	for _, queue := range c.assets {
		for _, p := range queue {
			p.done <- err
		}
	}
	c.assets = make(map[string][]*assetPending)
}
func (c *Client) Screenshot(ctx context.Context) (*ScreenshotData, error) {
	var r ScreenshotData
	if err := c.call(ctx, "screenshot", nil, &r, 0); err != nil {
		return nil, err
	}
	if r.DataURI == "" {
		return nil, errors.New("received screenshotResult without payload.dataUri")
	}
	return &r, nil
}
func (c *Client) GetElementTree(ctx context.Context, opts *ElementTreeOptions) (*ElementTreeData, error) {
	params := map[string]any{}
	timeout := 30 * time.Second
	if opts != nil {
		params["waitForIdleTimeoutMs"] = opts.WaitForIdleTimeout.Milliseconds()
		timeout += opts.WaitForIdleTimeout
	}
	r := ElementTreeData{Nodes: []ElementNode{}}
	err := c.call(ctx, "getElementTree", params, &r, timeout)
	return &r, err
}
func (c *Client) FindElement(ctx context.Context, selector Selector, limit ...int) (*FindElementResult, error) {
	n := 20
	if len(limit) > 0 {
		n = limit[0]
	}
	var r struct {
		Elements []ElementNode `json:"elements"`
		Count    *int          `json:"count"`
	}
	err := c.call(ctx, "findElement", map[string]any{"selector": selector, "limit": n}, &r, 0)
	count := len(r.Elements)
	if r.Count != nil {
		count = *r.Count
	}
	return &FindElementResult{r.Elements, count}, err
}
func (c *Client) Tap(ctx context.Context, target ElementTarget) (*TapResult, error) {
	var r TapResult
	err := c.call(ctx, "tap", target, &r, 0)
	return &r, err
}
func (c *Client) SetText(ctx context.Context, target *ElementTarget, text string) (*SetTextResult, error) {
	params := map[string]any{"text": text}
	if target != nil {
		if target.Selector != nil {
			params["selector"] = target.Selector
		}
		if target.X != nil {
			params["x"] = *target.X
		}
		if target.Y != nil {
			params["y"] = *target.Y
		}
	}
	r := SetTextResult{TextLength: len(utf16.Encode([]rune(text)))}
	err := c.call(ctx, "setText", params, &r, 0)
	return &r, err
}
func (c *Client) PressKey(ctx context.Context, key string, modifiers ...string) (*PressKeyResult, error) {
	params := map[string]any{"keyName": key}
	if modifiers != nil {
		params["modifiers"] = modifiers
	}
	r := PressKeyResult{Key: key}
	err := c.call(ctx, "pressKey", params, &r, 0)
	return &r, err
}
func (c *Client) ScrollScreen(ctx context.Context, direction ScrollDirection, amount ...float64) (*ScrollResult, error) {
	return c.scroll(ctx, "scrollScreen", nil, direction, amount)
}
func (c *Client) ScrollElement(ctx context.Context, target ElementTarget, direction ScrollDirection, amount ...float64) (*ScrollResult, error) {
	return c.scroll(ctx, "scrollElement", &target, direction, amount)
}
func (c *Client) scroll(ctx context.Context, kind string, target *ElementTarget, direction ScrollDirection, amount []float64) (*ScrollResult, error) {
	n := float64(6)
	if len(amount) > 0 {
		n = amount[0]
	}
	params := map[string]any{"direction": direction, "amount": n}
	if target != nil {
		if target.Selector != nil {
			params["selector"] = target.Selector
		}
		if target.X != nil {
			params["x"] = *target.X
		}
		if target.Y != nil {
			params["y"] = *target.Y
		}
	}
	var r ScrollResult
	err := c.call(ctx, kind, params, &r, 0)
	return &r, err
}
func (c *Client) OpenURL(ctx context.Context, url string) (*OpenURLResult, error) {
	r := OpenURLResult{URL: url}
	err := c.call(ctx, "openUrl", map[string]any{"url": url}, &r, 0)
	return &r, err
}
func (c *Client) LaunchApp(ctx context.Context, packageName string, opts *LaunchAppOptions) (*LaunchAppResult, error) {
	params := map[string]any{"packageName": packageName}
	id := ""
	if opts != nil {
		if opts.Mode != "" {
			params["mode"] = opts.Mode
		}
		if opts.OnExit != nil {
			id = control.ID()
			params["execId"] = id
			c.callbacks.Store(id, opts.OnExit)
		}
	}
	r := LaunchAppResult{PackageName: packageName}
	err := c.call(ctx, "launchApp", params, &r, 60*time.Second)
	if err != nil && id != "" {
		c.callbacks.Delete(id)
	}
	return &r, err
}
func (c *Client) TerminateApp(ctx context.Context, packageName string) error {
	return c.call(ctx, "terminateApp", map[string]any{"packageName": packageName}, nil, 0)
}
func (c *Client) ListApps(ctx context.Context) ([]InstalledApp, error) {
	var r struct {
		Apps []InstalledApp `json:"apps"`
	}
	err := c.call(ctx, "listApps", nil, &r, 0)
	return r.Apps, err
}

type AppWatch struct {
	ExecID string
	client *Client
}

func (w *AppWatch) Stop(ctx context.Context) error {
	w.client.callbacks.Delete(w.ExecID)
	return w.client.call(ctx, "unwatchApp", map[string]any{"execId": w.ExecID}, nil, 0)
}
func (c *Client) WatchApp(ctx context.Context, packageName string, onExit LaunchAppExitCallback) (*AppWatch, error) {
	id := control.ID()
	c.callbacks.Store(id, onExit)
	err := c.call(ctx, "watchApp", map[string]any{"packageName": packageName, "execId": id}, nil, 0)
	if err != nil {
		c.callbacks.Delete(id)
		return nil, err
	}
	return &AppWatch{id, c}, nil
}
func (c *Client) PlayOnMicrophone(ctx context.Context, path string, once ...bool) (*PlayOnMicrophoneResult, error) {
	if path == "" {
		return nil, errors.New("path must be a non-empty string")
	}
	params := map[string]any{"path": path}
	if len(once) > 0 {
		params["once"] = once[0]
	}
	var r PlayOnMicrophoneResult
	err := c.call(ctx, "playOnMicrophone", params, &r, 0)
	return &r, err
}
func (c *Client) SetWifiBandwidth(ctx context.Context, opts WifiBandwidthOptions) error {
	if opts.DownKbps == nil && opts.UpKbps == nil {
		return errors.New("setWifiBandwidth requires downKbps, upKbps, or both")
	}
	for field, value := range map[string]*int64{"downKbps": opts.DownKbps, "upKbps": opts.UpKbps} {
		if value != nil && *value < 0 {
			return fmt.Errorf("%s must be a non-negative integer Kbps value", field)
		}
	}
	return c.call(ctx, "setWifiBandwidth", opts, nil, 0)
}
func (c *Client) ADBShell(ctx context.Context, command string, args []string, opts *ADBShellOptions) (*ADBShellResult, error) {
	if command == "" {
		return nil, errors.New("command must be a non-empty string")
	}
	tokens := append([]string{command}, args...)
	for i, v := range tokens {
		tokens[i] = "'" + strings.ReplaceAll(v, "'", "'\\''") + "'"
	}
	params := map[string]any{"command": strings.Join(tokens, " ")}
	timeout := 30 * time.Second
	if opts != nil && opts.Timeout != 0 {
		timeout = opts.Timeout
		params["timeoutMs"] = timeout.Milliseconds()
	}
	if opts != nil && opts.TimeoutMS != nil {
		timeout = time.Duration(*opts.TimeoutMS) * time.Millisecond
		params["timeoutMs"] = *opts.TimeoutMS
	}
	var r struct {
		Stdout    string `json:"stdout"`
		Stderr    string `json:"stderr"`
		ExitCode  *int   `json:"exitCode"`
		Truncated bool   `json:"truncated"`
	}
	if err := c.call(ctx, "adbShell", params, &r, timeout+15*time.Second); err != nil {
		return nil, err
	}
	stdout, err := base64.StdEncoding.DecodeString(r.Stdout)
	if err != nil {
		return nil, err
	}
	stderr, err := base64.StdEncoding.DecodeString(r.Stderr)
	if err != nil {
		return nil, err
	}
	code := -1
	if r.ExitCode != nil {
		code = *r.ExitCode
	}
	encoding := ""
	if opts != nil {
		encoding = opts.Encoding
	}
	out, err := decodeShellOutput(stdout, encoding)
	if err != nil {
		return nil, err
	}
	stderrText, err := decodeShellOutput(stderr, encoding)
	if err != nil {
		return nil, err
	}
	return &ADBShellResult{out, stderrText, code, r.Truncated}, nil
}

func decodeShellOutput(data []byte, encoding string) (string, error) {
	encoding = strings.ToLower(encoding)
	switch encoding {
	case "", "utf8", "utf-8":
		return string([]rune(string(data))), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(data), nil
	case "base64url":
		return base64.RawURLEncoding.EncodeToString(data), nil
	case "hex":
		return hex.EncodeToString(data), nil
	case "utf16le", "utf-16le", "ucs2", "ucs-2":
		units := make([]uint16, len(data)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(data[2*i:])
		}
		return string(utf16.Decode(units)), nil
	case "latin1", "binary", "ascii":
		runes := make([]rune, len(data))
		for i, b := range data {
			if encoding == "ascii" {
				b &= 0x7f
			}
			runes[i] = rune(b)
		}
		return string(runes), nil
	default:
		return "", fmt.Errorf("unknown encoding: %s", encoding)
	}
}
