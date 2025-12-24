// Package ios provides a client for interacting with Limrun iOS instances
// via WebSocket connection. It supports all simulator control operations including
// screenshots, element interactions, typing, and more.
package ios

import (
	"context"
	"encoding/base64"
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

// AccessibilitySelector defines criteria for finding accessibility elements.
// All non-empty fields must match for an element to be selected.
type AccessibilitySelector struct {
	AccessibilityID string `json:"accessibilityId,omitempty"`
	Label           string `json:"label,omitempty"`
	LabelContains   string `json:"labelContains,omitempty"`
	ElementType     string `json:"elementType,omitempty"`
	Title           string `json:"title,omitempty"`
	TitleContains   string `json:"titleContains,omitempty"`
	Value           string `json:"value,omitempty"`
}

// AccessibilityPoint represents a point on the screen.
type AccessibilityPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ScreenshotData contains the result of a screenshot operation.
type ScreenshotData struct {
	Base64 string  // Base64-encoded JPEG image data
	Width  float64 // Width in points
	Height float64 // Height in points
}

// TapElementResult contains information about a tapped element.
type TapElementResult struct {
	ElementLabel string
	ElementType  string
}

// ElementResult contains information about an element after an operation.
type ElementResult struct {
	ElementLabel string
}

// InstalledApp represents an installed application on the simulator.
type InstalledApp struct {
	BundleID    string `json:"bundleId"`
	Name        string `json:"name"`
	InstallType string `json:"installType"`
}

// LsofEntry represents an open file entry.
type LsofEntry struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

// AppInstallationResult contains the result of a successful app installation.
type AppInstallationResult struct {
	URL      string // The URL the app was installed from
	BundleID string // Bundle ID of the installed app (always set on success)
}

// LaunchMode specifies how to launch an app after installation.
type LaunchMode string

const (
	// LaunchModeForegroundIfRunning brings the app to foreground if already running, otherwise launches it.
	LaunchModeForegroundIfRunning LaunchMode = "ForegroundIfRunning"
	// LaunchModeRelaunchIfRunning kills and relaunches the app if already running.
	LaunchModeRelaunchIfRunning LaunchMode = "RelaunchIfRunning"
	// LaunchModeFailIfRunning fails if the app is already running.
	LaunchModeFailIfRunning LaunchMode = "FailIfRunning"
)

// AppInstallationOptions configures app installation behavior.
type AppInstallationOptions struct {
	// MD5 hash for caching - if provided and matches cached version, skips download
	MD5 string
	// LaunchMode after installation. Leave empty to not launch after installation.
	LaunchMode LaunchMode
}

// Option configures a Client.
type Option func(*Client)

// WithLogger sets a custom logger. Defaults to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// Client is a WebSocket client for interacting with a Limrun iOS instance.
type Client struct {
	apiURL string
	token  string
	logger *slog.Logger

	ws               *websocket.Conn
	wsMu             sync.Mutex
	pendingRequests  sync.Map // map[string]chan *response
	simctlExecutions sync.Map // map[string]*SimctlCmd
	requestID        atomic.Uint64
	closed           atomic.Bool
	done             chan struct{}
}

// Orientation represents a device orientation.
type Orientation string

const (
	// OrientationPortrait sets the device to portrait mode.
	OrientationPortrait Orientation = "Portrait"
	// OrientationLandscape sets the device to landscape mode.
	OrientationLandscape Orientation = "Landscape"
)

// request is an internal type for WebSocket requests.
type request struct {
	Type        string                 `json:"type"`
	ID          string                 `json:"id"`
	X           float64                `json:"x,omitempty"`
	Y           float64                `json:"y,omitempty"`
	Point       *AccessibilityPoint    `json:"point,omitempty"`
	Selector    *AccessibilitySelector `json:"selector,omitempty"`
	Text        string                 `json:"text,omitempty"`
	PressEnter  bool                   `json:"pressEnter,omitempty"`
	Key         string                 `json:"key,omitempty"`
	Modifiers   []string               `json:"modifiers,omitempty"`
	BundleID    string                 `json:"bundleId,omitempty"`
	URL         string                 `json:"url,omitempty"`
	Kind        string                 `json:"kind,omitempty"`
	Args        []string               `json:"args,omitempty"`
	MD5         string                 `json:"md5,omitempty"`
	LaunchMode  LaunchMode             `json:"launchMode,omitempty"`
	Orientation Orientation            `json:"orientation,omitempty"`
}

// response is an internal type for handling WebSocket responses.
type response struct {
	Type         string          `json:"type"`
	ID           string          `json:"id"`
	Error        string          `json:"error,omitempty"`
	Base64       string          `json:"base64,omitempty"`
	Width        float64         `json:"width,omitempty"`
	Height       float64         `json:"height,omitempty"`
	JSON         string          `json:"json,omitempty"`
	ElementLabel string          `json:"elementLabel,omitempty"`
	ElementType  string          `json:"elementType,omitempty"`
	Apps         string          `json:"apps,omitempty"`
	Files        json.RawMessage `json:"files,omitempty"`
	URL          string          `json:"url,omitempty"`
	BundleID     string          `json:"bundleId,omitempty"`
	// simctlStream fields
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

// NewClient creates a new WebSocket client and connects to the given API URL.
func NewClient(apiURL, token string, opts ...Option) (*Client, error) {
	c := &Client{
		apiURL: apiURL,
		token:  token,
		logger: slog.Default(),
		done:   make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}

	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) connect() error {
	wsURL := strings.Replace(strings.Replace(c.apiURL, "https://", "wss://", 1), "http://", "ws://", 1)

	u, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("invalid API URL: %w", err)
	}
	u = u.JoinPath("signaling")
	q := u.Query()
	q.Set("token", c.token)
	u.RawQuery = q.Encode()

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), http.Header{})
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}

	c.wsMu.Lock()
	c.ws = ws
	c.wsMu.Unlock()

	go c.readLoop()
	go c.pingLoop()

	return nil
}

// Close closes the WebSocket connection and releases resources.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil // Already closed
	}
	close(c.done)

	c.wsMu.Lock()
	err := c.ws.Close()
	c.wsMu.Unlock()

	// Fail all pending requests
	c.pendingRequests.Range(func(key, value any) bool {
		close(value.(chan *response))
		c.pendingRequests.Delete(key)
		return true
	})

	// Fail all simctl executions
	c.simctlExecutions.Range(func(key, value any) bool {
		cmd := value.(*SimctlCmd)
		cmd.handleError(ErrConnectionClose)
		c.simctlExecutions.Delete(key)
		return true
	})

	return err
}

func (c *Client) readLoop() {
	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			if !c.closed.Load() {
				c.logger.Error("websocket read error", "error", err)
			}
			return
		}

		var resp response
		if err := json.Unmarshal(message, &resp); err != nil {
			c.logger.Error("failed to parse message", "error", err)
			continue
		}

		// Handle simctl streaming separately
		if resp.Type == "simctlStream" {
			if val, ok := c.simctlExecutions.Load(resp.ID); ok {
				cmd := val.(*SimctlCmd)
				var stdout, stderr []byte
				if resp.Stdout != "" {
					stdout, _ = base64.StdEncoding.DecodeString(resp.Stdout)
				}
				if resp.Stderr != "" {
					stderr, _ = base64.StdEncoding.DecodeString(resp.Stderr)
				}
				cmd.handleOutput(stdout, stderr, resp.ExitCode)
				if resp.ExitCode != nil {
					c.simctlExecutions.Delete(resp.ID)
				}
			}
			continue
		}

		if ch, ok := c.pendingRequests.LoadAndDelete(resp.ID); ok {
			ch.(chan *response) <- &resp
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

func (c *Client) sendRequest(ctx context.Context, req *request) (*response, error) {
	if c.closed.Load() {
		return nil, ErrNotConnected
	}

	req.ID = fmt.Sprintf("go-%d-%d", time.Now().UnixNano(), c.requestID.Add(1))
	respCh := make(chan *response, 1)
	c.pendingRequests.Store(req.ID, respCh)
	defer c.pendingRequests.Delete(req.ID)

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	c.logger.Debug("sending request", "type", req.Type, "id", req.ID)

	c.wsMu.Lock()
	err = c.ws.WriteMessage(websocket.TextMessage, data)
	c.wsMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp, ok := <-respCh:
		if !ok {
			return nil, ErrConnectionClose
		}
		if resp.Error != "" {
			return nil, errors.New(resp.Error)
		}
		return resp, nil
	}
}

// ============================================================================
// Client Methods
// ============================================================================

// Screenshot takes a screenshot of the current simulator screen.
func (c *Client) Screenshot(ctx context.Context) (*ScreenshotData, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "screenshot"})
	if err != nil {
		return nil, err
	}
	return &ScreenshotData{
		Base64: resp.Base64,
		Width:  resp.Width,
		Height: resp.Height,
	}, nil
}

// ElementTree returns the accessibility hierarchy of the current screen.
func (c *Client) ElementTree(ctx context.Context, point *AccessibilityPoint) (string, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "elementTree", Point: point})
	if err != nil {
		return "", err
	}
	return resp.JSON, nil
}

// Tap simulates a tap at the specified coordinates.
func (c *Client) Tap(ctx context.Context, x, y float64) error {
	_, err := c.sendRequest(ctx, &request{Type: "tap", X: x, Y: y})
	return err
}

// TapElement taps an accessibility element matching the selector.
func (c *Client) TapElement(ctx context.Context, selector AccessibilitySelector) (*TapElementResult, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "tapElement", Selector: &selector})
	if err != nil {
		return nil, err
	}
	return &TapElementResult{
		ElementLabel: resp.ElementLabel,
		ElementType:  resp.ElementType,
	}, nil
}

// IncrementElement increments an accessibility element (useful for sliders, steppers).
func (c *Client) IncrementElement(ctx context.Context, selector AccessibilitySelector) (*ElementResult, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "incrementElement", Selector: &selector})
	if err != nil {
		return nil, err
	}
	return &ElementResult{ElementLabel: resp.ElementLabel}, nil
}

// DecrementElement decrements an accessibility element (useful for sliders, steppers).
func (c *Client) DecrementElement(ctx context.Context, selector AccessibilitySelector) (*ElementResult, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "decrementElement", Selector: &selector})
	if err != nil {
		return nil, err
	}
	return &ElementResult{ElementLabel: resp.ElementLabel}, nil
}

// SetElementValue sets the value of an accessibility element.
func (c *Client) SetElementValue(ctx context.Context, text string, selector AccessibilitySelector) (*ElementResult, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "setElementValue", Text: text, Selector: &selector})
	if err != nil {
		return nil, err
	}
	return &ElementResult{ElementLabel: resp.ElementLabel}, nil
}

// TypeText types text into the currently focused input field.
func (c *Client) TypeText(ctx context.Context, text string, pressEnter bool) error {
	_, err := c.sendRequest(ctx, &request{Type: "typeText", Text: text, PressEnter: pressEnter})
	return err
}

// PressKey presses a key on the keyboard, optionally with modifiers.
func (c *Client) PressKey(ctx context.Context, key string, modifiers ...string) error {
	_, err := c.sendRequest(ctx, &request{Type: "pressKey", Key: key, Modifiers: modifiers})
	return err
}

// LaunchApp launches an installed app by bundle identifier.
func (c *Client) LaunchApp(ctx context.Context, bundleID string) error {
	_, err := c.sendRequest(ctx, &request{Type: "launchApp", BundleID: bundleID})
	return err
}

// ListApps returns a list of installed apps on the simulator.
func (c *Client) ListApps(ctx context.Context) ([]InstalledApp, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "listApps"})
	if err != nil {
		return nil, err
	}
	var apps []InstalledApp
	if err := json.Unmarshal([]byte(resp.Apps), &apps); err != nil {
		return nil, fmt.Errorf("parse apps: %w", err)
	}
	return apps, nil
}

// OpenURL opens a URL in the simulator.
func (c *Client) OpenURL(ctx context.Context, urlStr string) error {
	_, err := c.sendRequest(ctx, &request{Type: "openUrl", URL: urlStr})
	return err
}

// InstallApp installs an app from a URL (supports .ipa or .app files, optionally zipped).
// Returns the installation result with bundle ID on success.
func (c *Client) InstallApp(ctx context.Context, urlStr string, opts *AppInstallationOptions) (*AppInstallationResult, error) {
	req := &request{Type: "appInstallation", URL: urlStr}
	if opts != nil {
		req.MD5 = opts.MD5
		req.LaunchMode = opts.LaunchMode
	}
	resp, err := c.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	return &AppInstallationResult{
		URL:      resp.URL,
		BundleID: resp.BundleID,
	}, nil
}

// Lsof lists open Unix sockets on the instance.
func (c *Client) Lsof(ctx context.Context) ([]LsofEntry, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "listOpenFiles", Kind: "unix"})
	if err != nil {
		return nil, err
	}
	var files []LsofEntry
	if err := json.Unmarshal(resp.Files, &files); err != nil {
		return nil, fmt.Errorf("parse files: %w", err)
	}
	return files, nil
}

// SetOrientation sets the device orientation.
// Valid orientations are OrientationPortrait and OrientationLandscape.
func (c *Client) SetOrientation(ctx context.Context, orientation Orientation) error {
	_, err := c.sendRequest(ctx, &request{Type: "setOrientation", Orientation: orientation})
	return err
}

// Simctl creates a new SimctlCmd to run the given simctl arguments.
// The provided context is used to kill the process (by calling Kill)
// if the context becomes done before the command completes on its own.
//
// Example (similar to os/exec):
//
//	// Simple: capture output
//	output, err := client.Simctl(ctx, "listapps", "booted").Output()
//
//	// Stream output
//	cmd := client.Simctl(ctx, "launch", "booted", "com.example.app")
//	cmd.Stdout = os.Stdout
//	cmd.Stderr = os.Stderr
//	err := cmd.Run()
//
//	// With timeout (auto-kills when context expires)
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	output, err := client.Simctl(ctx, "spawn", "booted", "log", "stream").Output()
//
//	// With pipes
//	cmd := client.Simctl(ctx, "listapps", "booted")
//	stdout, _ := cmd.StdoutPipe()
//	cmd.Start()
//	io.Copy(os.Stdout, stdout)
//	cmd.Wait()
func (c *Client) Simctl(ctx context.Context, args ...string) *SimctlCmd {
	return &SimctlCmd{
		Args:   args,
		client: c,
		ctx:    ctx,
	}
}
