// Package ios provides a client for interacting with Limrun iOS instances
// via WebSocket connection. It supports all simulator control operations including
// screenshots, element interactions, typing, and more.
package ios

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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

// ElementTreeFrame is the bounding box of an accessibility element.
type ElementTreeFrame struct {
	Height float64 `json:"height"`
	Width  float64 `json:"width"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

// ElementTreeNode is a single node in the accessibility hierarchy.
type ElementTreeNode struct {
	AXFrame         string            `json:"AXFrame"`
	AXLabel         *string           `json:"AXLabel,omitempty"`
	AXUniqueID      *string           `json:"AXUniqueId,omitempty"`
	AXValue         *string           `json:"AXValue,omitempty"`
	Children        []ElementTreeNode `json:"children,omitempty"`
	ContentRequired bool              `json:"content_required"`
	CustomActions   []string          `json:"custom_actions"`
	Enabled         bool              `json:"enabled"`
	Frame           ElementTreeFrame  `json:"frame"`
	Help            *string           `json:"help,omitempty"`
	PID             int               `json:"pid"`
	Role            string            `json:"role"`
	RoleDescription string            `json:"role_description"`
	Subrole         *string           `json:"subrole,omitempty"`
	Title           *string           `json:"title,omitempty"`
	Traits          []string          `json:"traits"`
	Type            string            `json:"type"`
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

// WithHTTPClient sets a custom *http.Client used for the HTTP-based methods
// (Cp, StoreKit config, SoftReset, recording download). Defaults to
// http.DefaultClient.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// Client is a WebSocket client for interacting with a Limrun iOS instance.
type Client struct {
	apiURL     string
	token      string
	logger     *slog.Logger
	httpClient *http.Client

	keepAliveSessionID string

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

// ScrollDirection is the direction content moves during a scroll.
type ScrollDirection string

const (
	// ScrollUp moves content up.
	ScrollUp ScrollDirection = "up"
	// ScrollDown moves content down.
	ScrollDown ScrollDirection = "down"
	// ScrollLeft moves content left.
	ScrollLeft ScrollDirection = "left"
	// ScrollRight moves content right.
	ScrollRight ScrollDirection = "right"
)

// ScrollOptions configures a Scroll call.
type ScrollOptions struct {
	// Coordinate is the starting [x, y] of the gesture. Defaults to the
	// screen center when nil.
	Coordinate *[2]float64
	// Momentum controls scroll speed and inertia in the range 0.0-1.0.
	// 0 (default) is a slow scroll with no momentum; 1 is fastest with max
	// inertia.
	Momentum float64
}

// LaunchAppRuntime is an optional runtime injected during LaunchApp. Runtime
// launches always relaunch the app so injection is applied.
type LaunchAppRuntime struct {
	// Kind identifies the runtime, e.g. "detox".
	Kind string `json:"kind"`
	// ServerURL is the runtime's server URL.
	ServerURL string `json:"serverUrl"`
	// SessionID is the runtime session identifier.
	SessionID string `json:"sessionId"`
	// Version is the optional runtime version.
	Version string `json:"version,omitempty"`
}

// LaunchAppOption configures a LaunchApp call.
type LaunchAppOption func(*launchAppConfig)

type launchAppConfig struct {
	mode    LaunchMode
	runtime *LaunchAppRuntime
}

// WithLaunchMode sets the launch behavior when the app may already be running.
func WithLaunchMode(mode LaunchMode) LaunchAppOption {
	return func(c *launchAppConfig) { c.mode = mode }
}

// WithLaunchRuntime attaches a runtime to inject during launch. The launch is
// forced to RelaunchIfRunning so the runtime injection is applied.
func WithLaunchRuntime(runtime LaunchAppRuntime) LaunchAppOption {
	return func(c *launchAppConfig) { c.runtime = &runtime }
}

// CommandResult contains the result of a command execution (xcrun, xcodebuild).
type CommandResult struct {
	// Stdout is the decoded standard output of the command.
	Stdout string
	// Stderr is the decoded standard error of the command.
	Stderr string
	// ExitCode is the exit code of the command (-1 if unknown).
	ExitCode int
}

// DeviceInfo contains information about the simulator device.
type DeviceInfo struct {
	// UDID is the device UDID.
	UDID string
	// ScreenWidth is the screen width in points.
	ScreenWidth float64
	// ScreenHeight is the screen height in points.
	ScreenHeight float64
	// Model is the device model name.
	Model string
}

// RecordingOptions configures StartRecording.
type RecordingOptions struct {
	// Quality must be one of 5, 6, 7, 8, 9, 10. A zero value uses the server
	// default (5).
	Quality int
}

// SaveRecordingTo configures where StopRecording delivers the completed file.
type SaveRecordingTo struct {
	// PresignedURL, when set, makes the server upload the completed file there
	// before resolving.
	PresignedURL string
	// LocalPath, when set, makes the client download the completed file to that
	// path.
	LocalPath string
}

type recordingUpload struct {
	PresignedURL string `json:"presignedUrl"`
}

// request is an internal type for WebSocket requests.
type request struct {
	Type         string                 `json:"type"`
	ID           string                 `json:"id"`
	X            float64                `json:"x,omitempty"`
	Y            float64                `json:"y,omitempty"`
	ScreenWidth  float64                `json:"screenWidth,omitempty"`
	ScreenHeight float64                `json:"screenHeight,omitempty"`
	Point        *AccessibilityPoint    `json:"point,omitempty"`
	Selector     *AccessibilitySelector `json:"selector,omitempty"`
	Text         string                 `json:"text,omitempty"`
	PressEnter   bool                   `json:"pressEnter,omitempty"`
	Key          string                 `json:"key,omitempty"`
	Modifiers    []string               `json:"modifiers,omitempty"`
	BundleID     string                 `json:"bundleId,omitempty"`
	URL          string                 `json:"url,omitempty"`
	Kind         string                 `json:"kind,omitempty"`
	Args         []string               `json:"args,omitempty"`
	MD5          string                 `json:"md5,omitempty"`
	LaunchMode   LaunchMode             `json:"launchMode,omitempty"`
	Mode         LaunchMode             `json:"mode,omitempty"`
	Runtime      *LaunchAppRuntime      `json:"runtime,omitempty"`
	Orientation  Orientation            `json:"orientation,omitempty"`
	Lines        int                    `json:"lines,omitempty"`
	Direction    ScrollDirection        `json:"direction,omitempty"`
	Pixels       float64                `json:"pixels,omitempty"`
	Coordinate   []float64              `json:"coordinate,omitempty"`
	Momentum     float64                `json:"momentum,omitempty"`
	Actions      []PerformAction        `json:"actions,omitempty"`
	Quality      int                    `json:"quality,omitempty"`
	Upload       *recordingUpload       `json:"upload,omitempty"`
}

// response is an internal type for handling WebSocket responses.
type response struct {
	Type         string                `json:"type"`
	ID           string                `json:"id"`
	Error        string                `json:"error,omitempty"`
	Base64       string                `json:"base64,omitempty"`
	Width        float64               `json:"width,omitempty"`
	Height       float64               `json:"height,omitempty"`
	JSON         string                `json:"json,omitempty"`
	ElementLabel string                `json:"elementLabel,omitempty"`
	ElementType  string                `json:"elementType,omitempty"`
	Apps         string                `json:"apps,omitempty"`
	Files        json.RawMessage       `json:"files,omitempty"`
	URL          string                `json:"url,omitempty"`
	BundleID     string                `json:"bundleId,omitempty"`
	Logs         string                `json:"logs,omitempty"`
	Results      []PerformActionResult `json:"results,omitempty"`
	// deviceInfo fields
	UDID         string  `json:"udid,omitempty"`
	ScreenWidth  float64 `json:"screenWidth,omitempty"`
	ScreenHeight float64 `json:"screenHeight,omitempty"`
	Model        string  `json:"model,omitempty"`
	// simctlStream / command (xcrun, xcodebuild) fields
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

// NewClient creates a new WebSocket client and connects to the given API URL.
func NewClient(apiURL, token string, opts ...Option) (*Client, error) {
	c := &Client{
		apiURL:             apiURL,
		token:              token,
		logger:             slog.Default(),
		httpClient:         http.DefaultClient,
		keepAliveSessionID: newSessionID(),
		done:               make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}

	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

// signalingURL converts an http(s) API URL into the ws(s) signaling endpoint
// URL, including the auth token query parameter.
func signalingURL(apiURL, token string) (string, error) {
	wsURL := strings.Replace(strings.Replace(apiURL, "https://", "wss://", 1), "http://", "ws://", 1)

	u, err := url.Parse(wsURL)
	if err != nil {
		return "", fmt.Errorf("invalid API URL: %w", err)
	}
	u = u.JoinPath("signaling")
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// newSessionID returns a random hex identifier, falling back to a
// timestamp-based value if the system RNG is unavailable.
func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("go-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (c *Client) connect() error {
	wsURL, err := signalingURL(c.apiURL, c.token)
	if err != nil {
		return err
	}

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
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

// ElementTree returns the raw accessibility hierarchy JSON of the current
// screen. Pass a non-nil point to query the element at that specific location.
func (c *Client) ElementTree(ctx context.Context, point *AccessibilityPoint) (string, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "elementTree", Point: point})
	if err != nil {
		return "", err
	}
	return resp.JSON, nil
}

// ElementTreeNodes returns the parsed accessibility hierarchy of the current
// screen. Pass a non-nil point to query the element at that specific location.
func (c *Client) ElementTreeNodes(ctx context.Context, point *AccessibilityPoint) ([]ElementTreeNode, error) {
	raw, err := c.ElementTree(ctx, point)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var nodes []ElementTreeNode
	if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
		return nil, fmt.Errorf("parse element tree: %w", err)
	}
	return nodes, nil
}

// Tap simulates a tap at the specified coordinates, interpreted in the
// device's native screen dimensions.
func (c *Client) Tap(ctx context.Context, x, y float64) error {
	_, err := c.sendRequest(ctx, &request{Type: "tap", X: x, Y: y})
	return err
}

// TapWithScreenSize taps at coordinates given in an explicit coordinate space.
// Use this when coordinates are in a different coordinate space than the
// device's native dimensions.
func (c *Client) TapWithScreenSize(ctx context.Context, x, y, screenWidth, screenHeight float64) error {
	_, err := c.sendRequest(ctx, &request{Type: "tap", X: x, Y: y, ScreenWidth: screenWidth, ScreenHeight: screenHeight})
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

// ToggleKeyboard toggles the on-screen software keyboard visibility. This is
// equivalent to pressing Cmd+K in the iOS Simulator.
func (c *Client) ToggleKeyboard(ctx context.Context) error {
	_, err := c.sendRequest(ctx, &request{Type: "toggleKeyboard"})
	return err
}

// LaunchApp launches an installed app by bundle identifier.
//
// By default the app is brought to the foreground if already running. Pass
// WithLaunchMode to change this, or WithLaunchRuntime to inject a runtime
// (which forces RelaunchIfRunning).
func (c *Client) LaunchApp(ctx context.Context, bundleID string, opts ...LaunchAppOption) error {
	var cfg launchAppConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	mode := cfg.mode
	if cfg.runtime != nil {
		if cfg.mode == LaunchModeForegroundIfRunning {
			return errors.New("launchApp runtime launches require RelaunchIfRunning so runtime injection is applied")
		}
		mode = LaunchModeRelaunchIfRunning
	}
	_, err := c.sendRequest(ctx, &request{Type: "launchApp", BundleID: bundleID, Mode: mode, Runtime: cfg.runtime})
	return err
}

// TerminateApp terminates a running app by bundle identifier. It succeeds
// silently if the app is not currently running.
func (c *Client) TerminateApp(ctx context.Context, bundleID string) error {
	_, err := c.sendRequest(ctx, &request{Type: "terminateApp", BundleID: bundleID})
	return err
}

// AppLogTail returns the last N lines of an app's logs (combined stdout/stderr).
// The line count is clamped to the server limit.
func (c *Client) AppLogTail(ctx context.Context, bundleID string, lines int) (string, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "appLogTail", BundleID: bundleID, Lines: lines})
	if err != nil {
		return "", err
	}
	return resp.Logs, nil
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

// Scroll scrolls in the given direction by the specified number of pixels
// (the finger movement distance). Pass opts to set the starting coordinate or
// momentum; a nil opts uses the screen center with no momentum.
func (c *Client) Scroll(ctx context.Context, direction ScrollDirection, pixels float64, opts *ScrollOptions) error {
	req := &request{Type: "scroll", Direction: direction, Pixels: pixels}
	if opts != nil {
		if opts.Coordinate != nil {
			req.Coordinate = []float64{opts.Coordinate[0], opts.Coordinate[1]}
		}
		req.Momentum = opts.Momentum
	}
	_, err := c.sendRequest(ctx, req)
	return err
}

// StartRecording starts recording simulator video. Use StopRecording to stop.
// When opts.Quality is set it must be one of 5, 6, 7, 8, 9, or 10.
func (c *Client) StartRecording(ctx context.Context, opts *RecordingOptions) error {
	req := &request{Type: "startVideoRecording"}
	if opts != nil && opts.Quality != 0 {
		if opts.Quality < 5 || opts.Quality > 10 {
			return errors.New("quality must be one of: 5, 6, 7, 8, 9, 10")
		}
		req.Quality = opts.Quality
	}
	_, err := c.sendRequest(ctx, req)
	return err
}

// KeepAlive sends an application-level keepAlive message on the control
// websocket. It is fire-and-forget and does not wait for a response.
func (c *Client) KeepAlive() error {
	if c.closed.Load() {
		return ErrNotConnected
	}
	msg := struct {
		Type      string `json:"type"`
		SessionID string `json:"sessionId"`
	}{Type: "keepAlive", SessionID: c.keepAliveSessionID}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.wsMu.Lock()
	err = c.ws.WriteMessage(websocket.TextMessage, data)
	c.wsMu.Unlock()
	return err
}

// Xcrun runs an xcrun command with the given arguments and returns the
// complete output once it finishes (non-streaming).
//
// Only the following flags are allowed (validated server-side): --sdk <value>,
// --show-sdk-version, --show-sdk-build-version, --show-sdk-platform-version.
func (c *Client) Xcrun(ctx context.Context, args ...string) (*CommandResult, error) {
	return c.runCommand(ctx, "xcrun", args)
}

// Xcodebuild runs an xcodebuild command with the given arguments and returns
// the complete output once it finishes (non-streaming). Only -version is
// allowed (validated server-side).
func (c *Client) Xcodebuild(ctx context.Context, args ...string) (*CommandResult, error) {
	return c.runCommand(ctx, "xcodebuild", args)
}

func (c *Client) runCommand(ctx context.Context, msgType string, args []string) (*CommandResult, error) {
	resp, err := c.sendRequest(ctx, &request{Type: msgType, Args: args})
	if err != nil {
		return nil, err
	}
	stdout, err := decodeBase64(resp.Stdout)
	if err != nil {
		return nil, fmt.Errorf("decode stdout: %w", err)
	}
	stderr, err := decodeBase64(resp.Stderr)
	if err != nil {
		return nil, fmt.Errorf("decode stderr: %w", err)
	}
	exitCode := -1
	if resp.ExitCode != nil {
		exitCode = *resp.ExitCode
	}
	return &CommandResult{Stdout: string(stdout), Stderr: string(stderr), ExitCode: exitCode}, nil
}

func decodeBase64(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

// DeviceInfo fetches information about the simulator device (UDID, screen
// dimensions, and model).
func (c *Client) DeviceInfo(ctx context.Context) (*DeviceInfo, error) {
	resp, err := c.sendRequest(ctx, &request{Type: "deviceInfo"})
	if err != nil {
		return nil, err
	}
	return &DeviceInfo{
		UDID:         resp.UDID,
		ScreenWidth:  resp.ScreenWidth,
		ScreenHeight: resp.ScreenHeight,
		Model:        resp.Model,
	}, nil
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
