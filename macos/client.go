// Package macos works inside a running macOS instance: it runs shell commands
// as the guest's user, forwards local ports into the guest, and drives the
// screen with the mouse and keyboard.
package macos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/limrun-inc/go-sdk/build"
	"github.com/limrun-inc/go-sdk/internal/direct"
	"github.com/limrun-inc/go-sdk/internal/stream"
	"github.com/limrun-inc/go-sdk/session"
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

// RestoreSnapshotOptions configures RestoreSnapshot.
type RestoreSnapshotOptions struct {
	// Key is the asset name the instance saves its changes under when it
	// terminates. It is set once.
	Key string
	// RestoreKeys are asset names, or name prefixes, to restore from, in order.
	// They default to Key.
	RestoreKeys []string
	// OnProgress is called for every progress event the instance reports.
	OnProgress func(SnapshotProgress)
}

// SnapshotProgress is a restore's progress, as the instance reports it.
type SnapshotProgress struct {
	// Phase is one of "resolving", "downloading", "extracting", "restored",
	// "skipped" or "failed". New values may be added over time.
	Phase string `json:"phase"`
	// Key is the asset name the restore matched.
	Key string `json:"key,omitempty"`
	// MatchKind is "exact_hit" or "prefix_hit".
	MatchKind string `json:"matchKind,omitempty"`
	// Source tells whether the bytes came through the regional accelerator
	// ("tag") or straight from storage ("direct").
	Source     string `json:"source,omitempty"`
	TotalBytes int64  `json:"totalBytes,omitempty"`
	DoneBytes  int64  `json:"doneBytes,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
}

// RestoreSnapshot restores a disk snapshot into the guest and sets the key the
// instance saves under when it terminates. A snapshot is an asset of kind
// MacOSSnapshot, so the instance's role needs read on the restore keys' asset
// names, and create, upload and update on Key's.
//
// Each restore key is tried in order: its exact name first, then the newest
// asset whose name starts with it, skipping snapshots saved on another macOS
// build. Without RestoreKeys, Key is restored. Without either, nothing is
// restored. Call it once the instance is ready and before using the guest;
// asking again joins the restore already running. It returns the final
// progress event, and OnProgress sees every event, with byte counts while
// downloading. A stream that ends before a terminal phase (restored, skipped
// or failed) is an error, returned with the last event seen.
func (c *Client) RestoreSnapshot(ctx context.Context, opts RestoreSnapshotOptions) (*SnapshotProgress, error) {
	body, err := json.Marshal(struct {
		Key         string   `json:"key,omitempty"`
		RestoreKeys []string `json:"restoreKeys,omitempty"`
	}{opts.Key, opts.RestoreKeys})
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(ctx, http.MethodPost, c.http.URL+"/snapshot", bytes.NewReader(body), "application/json", -1)
	if err != nil {
		return nil, withReason(err)
	}
	defer res.Body.Close()
	var last *SnapshotProgress
	err = stream.Read(res.Body, func(e stream.Event) error {
		if e.Type != "snapshot" {
			return nil
		}
		var progress SnapshotProgress
		if err := json.Unmarshal([]byte(e.Data), &progress); err != nil {
			return fmt.Errorf("decoding snapshot progress %q: %w", e.Data, err)
		}
		last = &progress
		if opts.OnProgress != nil {
			opts.OnProgress(progress)
		}
		return nil
	}, func() {}, func(time.Duration) {})
	if err != nil && !errors.Is(err, io.EOF) {
		return last, err
	}
	switch phase := lastPhase(last); phase {
	case "restored", "skipped", "failed":
		return last, nil
	default:
		return last, fmt.Errorf("the snapshot stream ended before the restore did (last phase: %s)", phase)
	}
}

func lastPhase(p *SnapshotProgress) string {
	if p == nil {
		return "none"
	}
	return p.Phase
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

// TunnelOptions configures StartTunnel. As on iOS and Android, HTTP and HTTPS
// through the tunnel are inspected when Inspection is nil.
type TunnelOptions = tunnel.DestinationOptions

// TunnelStatus is the instance's tunnel as GetTunnelStatus reports it.
type TunnelStatus = tunnel.DestinationStatus

// StartTunnel lets code in the guest reach destinations on this machine
// through a destination tunnel, as on an iOS simulator. localhost:port and
// IPv4:port selectors are reached at those addresses in the guest; domain
// selectors resolve in the guest to addresses the tunnel serves and are dialed
// from this machine. One tunnel serves an instance at a time.
func (c *Client) StartTunnel(ctx context.Context, opts TunnelOptions) (*tunnel.Destination, error) {
	remote, err := tunnel.DestinationURL(c.http.URL)
	if err != nil {
		return nil, err
	}
	if opts.Inspection == nil {
		opts.Inspection = &tunnel.InspectionConfig{Enabled: true}
	}
	if opts.WebSocketDialer == nil {
		opts.WebSocketDialer = c.dialer
	}
	return tunnel.StartDestinationTCP(ctx, remote, c.http.Token, opts)
}

// GetTunnelStatus reports the instance's tunnel, if one is active.
func (c *Client) GetTunnelStatus(ctx context.Context) (*TunnelStatus, error) {
	return tunnel.GetDestinationStatus(ctx, c.http.URL, c.http.Token, c.http.HTTPClient)
}

// StopTunnel stops the tunnel with this ID; a tunnel that replaced it keeps running.
func (c *Client) StopTunnel(ctx context.Context, id string) error {
	return tunnel.StopDestination(ctx, c.http.URL, c.http.Token, id, c.http.HTTPClient)
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

// TrustedCACertificate is a CA certificate the guest now trusts.
type TrustedCACertificate = direct.TrustedCACertificate

// AddCACertificate trusts a PEM-encoded CA certificate system-wide in the
// guest, for example the CA of an intercepting proxy you record traffic with.
// macOS asks an administrator to confirm a new trusted root, so the instance
// answers that dialog itself, and it shows on the screen for a few seconds.
// Adding a certificate the guest already trusts changes nothing.
func (c *Client) AddCACertificate(ctx context.Context, pem string) (*TrustedCACertificate, error) {
	trusted, err := c.http.AddCACertificate(ctx, pem)
	return trusted, withReason(err)
}

// PersistOption persists a capture to Limrun's bucket after it stops; a nil
// TTLSeconds keeps it for the server default, 72 hours.
type PersistOption = session.PersistOption

// RecordingOptions configures StartRecording.
type RecordingOptions struct {
	// RecordingID matches [a-z0-9][a-z0-9_-]{0,63}. Generated when empty; never reused.
	RecordingID string
	// Quality is the encoder quality from 5 to 10; the server default is 5.
	Quality *int
	// Persist uploads the recording when it stops; list it with ListRecordings.
	Persist *PersistOption
}

// SaveRecordingTo says where a stopped recording goes besides the instance's
// own download URL.
type SaveRecordingTo struct {
	// PresignedURL is an HTTPS PUT URL the instance uploads the MP4 to before the stop returns.
	PresignedURL string
	// LocalPath is a file this client downloads the MP4 to.
	LocalPath string
}

// StartRecording starts recording the screen and returns the recording's ID.
// Up to eight recordings may be active or uploading at once.
func (c *Client) StartRecording(ctx context.Context, opts *RecordingOptions) (string, error) {
	body := map[string]any{}
	if opts != nil {
		if opts.RecordingID != "" {
			if err := validateRecordingID(opts.RecordingID); err != nil {
				return "", err
			}
			body["recordingId"] = opts.RecordingID
		}
		if opts.Quality != nil {
			body["quality"] = *opts.Quality
		}
		fields, err := opts.Persist.Fields()
		if err != nil {
			return "", err
		}
		for k, v := range fields {
			body[k] = v
		}
	}
	var out struct {
		RecordingID string `json:"recordingId"`
	}
	if err := c.request(ctx, http.MethodPost, "recordings", body, &out); err != nil {
		return "", err
	}
	return out.RecordingID, nil
}

// StopRecording stops the recording with this ID, or the latest active one
// when id is empty, and returns its download URL, which takes the instance
// token and lasts while the instance runs.
func (c *Client) StopRecording(ctx context.Context, id string, saveTo *SaveRecordingTo) (string, error) {
	path := "recordings/stop"
	if id != "" {
		if err := validateRecordingID(id); err != nil {
			return "", err
		}
		path = "recordings/" + id + "/stop"
	}
	body := map[string]any{}
	if saveTo != nil && saveTo.PresignedURL != "" {
		body["presignedUrl"] = saveTo.PresignedURL
	}
	var out struct {
		RecordingID string `json:"recordingId"`
	}
	if err := c.request(ctx, http.MethodPost, path, body, &out); err != nil {
		return "", err
	}
	if out.RecordingID == "" {
		out.RecordingID = id
	}
	if out.RecordingID == "" {
		return "", errors.New("the instance did not say which recording stopped")
	}
	downloadURL := c.http.URL + "/files?" + url.Values{"name": {"recording_" + out.RecordingID + ".mp4"}}.Encode()
	if saveTo != nil && saveTo.LocalPath != "" {
		if err := c.http.Download(ctx, downloadURL, saveTo.LocalPath); err != nil {
			return "", withReason(err)
		}
	}
	return downloadURL, nil
}

func validateRecordingID(id string) error {
	if !recordingIDPattern.MatchString(id) {
		return errors.New("recordingId must match ^[a-z0-9][a-z0-9_-]{0,63}$")
	}
	return nil
}

var recordingIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// AppLogTail returns the last lines an app logged to the unified log in the past hour.
func (c *Client) AppLogTail(ctx context.Context, bundleID string, lines int) (string, error) {
	var out struct {
		Logs string `json:"logs"`
	}
	path := "app-logs?" + url.Values{"bundleId": {bundleID}, "lines": {strconv.Itoa(lines)}}.Encode()
	if err := c.request(ctx, http.MethodGet, path, nil, &out); err != nil {
		return "", err
	}
	return out.Logs, nil
}

// StreamAppLog follows an app's unified log as it is written, one message per entry.
func (c *Client) StreamAppLog(ctx context.Context, bundleID string) *session.Stream[string] {
	return session.StartTextStream(ctx, c.http.HTTPClient,
		c.http.URL+"/app-logs/stream?"+url.Values{"bundleId": {bundleID}}.Encode(), c.http.Token)
}

// StartAppLogCapture starts capturing an app's logs. One app log capture runs
// at a time. With persist, the capture is uploaded when it stops or the
// instance terminates; list it with ListAppLogs.
func (c *Client) StartAppLogCapture(ctx context.Context, bundleID string, persist *PersistOption) error {
	if bundleID == "" {
		return errors.New("bundleId must be a non-empty string")
	}
	params, err := persist.Fields()
	if err != nil {
		return err
	}
	params["bundleId"] = bundleID
	return c.request(ctx, http.MethodPost, "session/applogs/start", params, nil)
}

// StopAppLogCapture stops the active app log capture.
func (c *Client) StopAppLogCapture(ctx context.Context) error {
	return c.request(ctx, http.MethodPost, "session/applogs/stop", map[string]any{}, nil)
}

// StartEventCapture starts capturing the coalesced event log of clicks,
// scrolls and keys sent to the screen. Typing is logged as a count of
// characters, never their content.
func (c *Client) StartEventCapture(ctx context.Context, persist *PersistOption) error {
	params, err := persist.Fields()
	if err != nil {
		return err
	}
	return c.request(ctx, http.MethodPost, "session/events/start", params, nil)
}

// StopEventCapture stops the active event capture.
func (c *Client) StopEventCapture(ctx context.Context) error {
	return c.request(ctx, http.MethodPost, "session/events/stop", map[string]any{}, nil)
}

// StartPerformanceCapture starts saving the running apps' performance samples.
// The capture always persists, kept for ttlSeconds or the server default of
// 72 hours when nil; list it with ListPerformanceCaptures.
func (c *Client) StartPerformanceCapture(ctx context.Context, ttlSeconds *int64) error {
	params := map[string]any{}
	if ttlSeconds != nil {
		if *ttlSeconds <= 0 {
			return errors.New("ttlSeconds must be a positive integer")
		}
		params["ttlSeconds"] = *ttlSeconds
	}
	return c.request(ctx, http.MethodPost, "session/performance/start", params, nil)
}

// StopPerformanceCapture stops the active performance capture and uploads it.
func (c *Client) StopPerformanceCapture(ctx context.Context) error {
	return c.request(ctx, http.MethodPost, "session/performance/stop", map[string]any{}, nil)
}

// StreamAppLogCapture tails the live app log capture. On connect the instance
// replays its recent lines, and a dropped connection resumes where it left off.
func (c *Client) StreamAppLogCapture(ctx context.Context) *session.Stream[session.LogLine] {
	return session.StartStream[session.LogLine](ctx, c.http.HTTPClient, c.http.URL+"/session/applogs/events", c.http.Token)
}

// StreamEventCapture tails the live event capture.
func (c *Client) StreamEventCapture(ctx context.Context) *session.Stream[session.Event] {
	return session.StartStream[session.Event](ctx, c.http.HTTPClient, c.http.URL+"/session/events/events", c.http.Token)
}

// StreamPerformance tails the running apps' samples, about once a second; no
// capture is needed. On connect the instance replays its recent samples.
func (c *Client) StreamPerformance(ctx context.Context) *session.Stream[session.PerformanceSample] {
	return session.StartStream[session.PerformanceSample](ctx, c.http.HTTPClient, c.http.URL+"/session/performance/events", c.http.Token)
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
