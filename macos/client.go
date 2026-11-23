// Package macos works inside a running macOS instance: it runs shell commands
// as the guest's user, forwards local ports into the guest, and drives the
// screen with the mouse and keyboard.
package macos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/limrun-inc/go-sdk/build"
	"github.com/limrun-inc/go-sdk/internal/direct"
	"github.com/limrun-inc/go-sdk/tunnel"
)

// RunOptions sets the working directory, environment and timeout of Run.
// Cwd is relative to the guest user's home directory and defaults to ".".
// Env entries have the form KEY=VALUE.
type RunOptions = build.RunOptions

// Point is a position on the screen, in pixels of Screenshot, from the
// top-left corner.
type Point struct {
	X, Y int
}

// Button is the mouse button Tap clicks.
type Button string

const (
	ButtonLeft  Button = "left"
	ButtonRight Button = "right"
)

// Modifier is a key held while PressKey presses a key.
type Modifier string

const (
	ModifierShift   Modifier = "shift"
	ModifierControl Modifier = "control"
	ModifierOption  Modifier = "option"
	ModifierCommand Modifier = "command"
)

// SSHAccess tells where the guest's SSH server can be reached once EnableSSH
// returns.
type SSHAccess struct {
	// User is the guest account the key logs in as.
	User string `json:"user"`
	// Port is the guest port sshd listens on; forward a local port to it with
	// StartPortForward.
	Port int `json:"port"`
}

// PortForwardOptions configures StartPortForward. Host is the local address to
// listen on and defaults to 127.0.0.1. The tunnel runs in multiplexed mode
// unless Mode says otherwise, and uses the client's WebSocket dialer unless
// WebSocketDialer is set.
type PortForwardOptions struct {
	Host string
	tunnel.TCPOptions
}

// TapOptions configures Tap. Button defaults to the left button.
type TapOptions struct {
	Button Button
}

// ScrollOptions configures Scroll. A positive DY scrolls down and a positive
// DX scrolls right, in pixels. At is the point to scroll over; when nil the
// scroll happens wherever the pointer is.
type ScrollOptions struct {
	DX, DY int
	At     *Point
}

// DragOptions configures Drag. DurationMs is how long the move from start to
// end takes; when nil the server picks.
type DragOptions struct {
	DurationMs *int
}

// Client works inside a running macOS instance.
type Client struct {
	daemon *build.DaemonClient
	http   direct.Client
	dialer *websocket.Dialer
}

// NewClient connects to a macOS instance with its API URL and token. Options
// such as build.WithHTTPClient, build.WithWebSocketDialer and
// build.WithExecutionOptions (for Run's output writers) apply as for the
// Xcode and Gradle clients.
func NewClient(apiURL, token string, opts ...build.Option) *Client {
	apiURL = strings.TrimRight(apiURL, "/")
	var o build.ExecOptions
	for _, f := range opts {
		f(&o)
	}
	return &Client{
		daemon: build.NewDaemonClient(apiURL, token, opts...),
		http:   direct.Client{URL: apiURL, Token: token, HTTPClient: o.HTTPClient},
		dialer: o.WebSocketDialer,
	}
}

// Run runs a shell command line as the guest's user, in its login session,
// with a login shell's PATH. Output goes to the writers set with
// build.WithExecutionOptions; Wait returns the exit code. Kill ends the
// command and everything it started.
func (c *Client) Run(ctx context.Context, commandLine string, opts *RunOptions) (*build.Process, error) {
	return c.daemon.Run(ctx, commandLine, opts)
}

// EnableSSH turns on the guest's SSH server and lets publicKey (one OpenSSH
// public key line) log in. Password logins stay off. Reach the server with
// StartPortForward(ctx, localPort, access.Port, nil).
func (c *Client) EnableSSH(ctx context.Context, publicKey string) (*SSHAccess, error) {
	var access SSHAccess
	if err := c.request(ctx, http.MethodPost, "ssh", map[string]string{"publicKey": publicKey}, &access); err != nil {
		return nil, err
	}
	return &access, nil
}

// SnapshotStatus is the disk snapshot status a macOS instance reports itself.
type SnapshotStatus struct {
	Restore struct {
		Phase   string `json:"phase"`
		Reason  string `json:"reason,omitempty"`
		Message string `json:"message,omitempty"`
		Key     string `json:"key,omitempty"`
	} `json:"restore"`
	Save struct {
		Phase   string `json:"phase"`
		Reason  string `json:"reason,omitempty"`
		Message string `json:"message,omitempty"`
	} `json:"save"`
}

// StartSnapshotRestore starts restoring the disk snapshot the instance was
// created with. The instance restores only when a client asks, so call it once
// the instance is ready and before using the guest; asking again changes
// nothing. MacosInstances.RestoreSnapshot also waits for the restore to end.
func (c *Client) StartSnapshotRestore(ctx context.Context) (*SnapshotStatus, error) {
	var status SnapshotStatus
	if err := c.request(ctx, http.MethodPost, "snapshot/restore", map[string]string{}, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// DerivePortForwardURL returns the WebSocket URL that carries TCP connections
// to remotePort inside the guest.
func DerivePortForwardURL(apiURL string, remotePort int) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported apiUrl protocol for port forwarding: %s", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/port-forward"
	u.RawPath = ""
	u.RawQuery = url.Values{"port": []string{strconv.Itoa(remotePort)}}.Encode()
	u.Fragment = ""
	return u.String(), nil
}

// StartPortForward accepts TCP connections on localPort (on 127.0.0.1 unless
// opts.Host is set) and carries each one to remotePort inside the guest. A
// localPort of 0 picks a free port; read it from the tunnel's Address.
func (c *Client) StartPortForward(ctx context.Context, localPort, remotePort int, opts *PortForwardOptions) (*tunnel.TCP, error) {
	if err := tunnel.AssertPort(remotePort, "remotePort", 1, 65535); err != nil {
		return nil, err
	}
	var o PortForwardOptions
	if opts != nil {
		o = *opts
	}
	if o.Mode == "" {
		o.Mode = "multiplexed"
	}
	if o.WebSocketDialer == nil {
		o.WebSocketDialer = c.dialer
	}
	remote, err := DerivePortForwardURL(c.http.URL, remotePort)
	if err != nil {
		return nil, err
	}
	return tunnel.StartTCP(ctx, remote, c.http.Token, o.Host, localPort, o.TCPOptions)
}

// Screenshot returns a JPEG of the screen. Its pixels are the coordinates the
// input methods take.
func (c *Client) Screenshot(ctx context.Context) ([]byte, error) {
	res, err := c.http.Do(ctx, http.MethodGet, c.http.URL+"/screenshot.jpg", nil, "", -1)
	if err != nil {
		return nil, withReason(err)
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

// Tap clicks at a point.
func (c *Client) Tap(ctx context.Context, point Point, opts *TapOptions) error {
	body := map[string]any{"x": point.X, "y": point.Y}
	if opts != nil && opts.Button != "" {
		body["button"] = opts.Button
	}
	return c.request(ctx, http.MethodPost, "tap", body, nil)
}

// TypeText types text with key presses on the US layout.
func (c *Client) TypeText(ctx context.Context, text string) error {
	return c.request(ctx, http.MethodPost, "type", map[string]string{"text": text}, nil)
}

// PressKey presses one key, such as "enter", "tab", "escape", "up", "f5" or
// "a", while holding the given modifiers.
func (c *Client) PressKey(ctx context.Context, key string, modifiers ...Modifier) error {
	body := map[string]any{"key": key}
	if len(modifiers) > 0 {
		body["modifiers"] = modifiers
	}
	return c.request(ctx, http.MethodPost, "press-key", body, nil)
}

// Scroll scrolls by DY and DX pixels over opts.At, or wherever the pointer is
// when At is nil.
func (c *Client) Scroll(ctx context.Context, opts ScrollOptions) error {
	body := map[string]any{"dx": opts.DX, "dy": opts.DY}
	if opts.At != nil {
		body["x"] = opts.At.X
		body["y"] = opts.At.Y
	}
	return c.request(ctx, http.MethodPost, "scroll", body, nil)
}

// Drag presses at from, moves to to over opts.DurationMs, and releases.
func (c *Client) Drag(ctx context.Context, from, to Point, opts *DragOptions) error {
	body := map[string]any{"fromX": from.X, "fromY": from.Y, "toX": to.X, "toY": to.Y}
	if opts != nil && opts.DurationMs != nil {
		body["durationMs"] = *opts.DurationMs
	}
	return c.request(ctx, http.MethodPost, "drag", body, nil)
}

func (c *Client) request(ctx context.Context, method, path string, params, out any) error {
	return withReason(c.http.JSON(ctx, method, path, params, out))
}

// withReason replaces a JSON error body with the reason it carries, so the
// error reads as the server's message rather than raw JSON.
func withReason(err error) error {
	var he *direct.HTTPError
	if !errors.As(err, &he) {
		return err
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal([]byte(he.Body), &body) == nil && body.Reason != "" {
		he.Body = body.Reason
	}
	return err
}
