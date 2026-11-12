package android

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"time"

	"github.com/limrun-inc/go-sdk/internal/control"
	"github.com/limrun-inc/go-sdk/internal/direct"
	"github.com/limrun-inc/go-sdk/session"
	"github.com/limrun-inc/go-sdk/tunnel"
)

func (c *Client) PushFile(ctx context.Context, path string, destination ...string) (string, error) {
	u := c.http.URL + "/files"
	if len(destination) > 0 {
		u += "?path=" + url.QueryEscape(destination[0])
	}
	return c.http.Upload(ctx, u, path)
}
func (c *Client) PullFile(ctx context.Context, remotePath, localPath string) error {
	return c.http.Download(ctx, c.http.URL+"/files?path="+url.QueryEscape(remotePath), localPath)
}
func (c *Client) SetCameraVideo(ctx context.Context, path string, loop ...bool) error {
	remote, err := c.PushFile(ctx, path)
	if err != nil {
		return err
	}
	repeat := true
	if len(loop) > 0 {
		repeat = loop[0]
	}
	return c.call(ctx, "cameraControl", map[string]any{"action": "setSource", "source": "video", "arg": remote, "loop": repeat}, nil, 0)
}
func (c *Client) ClearCameraVideo(ctx context.Context) error {
	return c.call(ctx, "cameraControl", map[string]any{"action": "reset"}, nil, 0)
}

// SendAsset installs an APK from a download URL. Older servers may correlate replies by URL.
func (c *Client) SendAsset(ctx context.Context, url string, timeout ...time.Duration) error {
	return c.sendAsset(ctx, "url", url, timeout...)
}

// SendAssetByName installs an APK using the instance's delegated asset read permission.
// It sends the exact asset name to the instance without calling Asset Storage.
func (c *Client) SendAssetByName(ctx context.Context, assetName string, timeout ...time.Duration) error {
	if assetName == "" {
		return errors.New("assetName must not be empty")
	}
	return c.sendAsset(ctx, "assetName", assetName, timeout...)
}

func (c *Client) sendAsset(ctx context.Context, field, value string, timeout ...time.Duration) error {
	wait := 120 * time.Second
	if len(timeout) > 0 {
		wait = timeout[0]
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	p := &assetPending{id: control.ID(), done: make(chan error, 1)}
	if field == "url" {
		p.url = value
	}
	defer func() {
		c.assetsMu.Lock()
		defer c.assetsMu.Unlock()
		for i, pending := range c.assets {
			if pending == p {
				c.assets = slices.Delete(c.assets, i, i+1)
				break
			}
		}
	}()
	// Keep registration order equal to wire order for legacy URL-only replies.
	c.assetsMu.Lock()
	c.assets = append(c.assets, p)
	err := c.transport.Send(map[string]any{"type": "asset", "id": p.id, field: value})
	c.assetsMu.Unlock()
	if err != nil {
		return err
	}
	select {
	case err := <-p.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *Client) StartADBTunnel() (*tunnel.ADB, error) {
	if c.adbURL == "" {
		return nil, errors.New("adbUrl is required to start an ADB tunnel")
	}
	t, err := tunnel.NewADB(c.adbURL, c.http.Token, tunnel.WithADBPath(c.adbPath), tunnel.WithADBConnectionOptions(tunnel.TCPOptions{WebSocketDialer: c.connectionOptions.Dialer, MaxReconnectAttempts: &c.connectionOptions.MaxReconnectAttempts, ReconnectDelay: c.connectionOptions.ReconnectDelay, MaxReconnectDelay: c.connectionOptions.MaxReconnectDelay}))
	if err != nil {
		return nil, err
	}
	if err = t.Start(); err != nil {
		t.Close()
		return nil, err
	}
	return t, nil
}

type TrustedCACertificate = direct.TrustedCACertificate

func (c *Client) AddCACertificate(ctx context.Context, pem string) (*TrustedCACertificate, error) {
	if c.adbURL == "" {
		return nil, errors.New("adbUrl is required to add a CA certificate")
	}
	u, err := url.Parse(c.adbURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "ws" {
		u.Scheme = "http"
	}
	if u.Scheme == "wss" {
		u.Scheme = "https"
	}
	u.RawQuery = ""
	u.Fragment = ""
	h := c.http
	h.URL = u.String()
	return h.AddCACertificate(ctx, pem)
}
func recordingFields(opts *RecordingOptions) (map[string]any, error) {
	if opts == nil {
		return map[string]any{}, nil
	}
	m, err := opts.Persist.Fields()
	if err != nil {
		return nil, err
	}
	if opts.Quality != 0 {
		if opts.Quality < 5 || opts.Quality > 10 {
			return nil, errors.New("quality must be one of: 5, 6, 7, 8, 9, 10")
		}
		m["quality"] = opts.Quality
	}
	return m, nil
}

var recordingIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func validateRecordingID(id string) error {
	if !recordingIDPattern.MatchString(id) {
		return errors.New("recordingId must be 1–64 lowercase letters, digits, underscores or hyphens, start with a letter or digit")
	}
	return nil
}
func (c *Client) StartRecording(ctx context.Context, opts *RecordingOptions) error {
	params, err := recordingFields(opts)
	if err != nil {
		return err
	}
	return c.call(ctx, "startRecording", params, nil, 0)
}
func (c *Client) StartRecordingSession(ctx context.Context, opts *RecordingOptions) (string, error) {
	params, err := recordingFields(opts)
	if err != nil {
		return "", err
	}
	if opts != nil && opts.RecordingID != "" {
		if err = validateRecordingID(opts.RecordingID); err != nil {
			return "", err
		}
		params["recordingId"] = opts.RecordingID
	}
	var r struct {
		RecordingID string `json:"recordingId"`
	}
	if err = c.call(ctx, "startRecordingSession", params, &r, 0); err != nil {
		return "", err
	}
	return r.RecordingID, validateRecordingID(r.RecordingID)
}
func (c *Client) StopRecording(ctx context.Context, saveTo *RecordingDestination) (string, error) {
	return c.stopRecording(ctx, "stopRecording", "", saveTo)
}
func (c *Client) StopRecordingSession(ctx context.Context, id string, saveTo *RecordingDestination) (string, error) {
	return c.stopRecording(ctx, "stopRecordingSession", id, saveTo)
}
func (c *Client) stopRecording(ctx context.Context, kind, id string, saveTo *RecordingDestination) (string, error) {
	params := map[string]any{}
	if id != "" {
		if err := validateRecordingID(id); err != nil {
			return "", err
		}
		params["recordingId"] = id
	}
	if saveTo != nil && saveTo.PresignedURL != "" {
		params["upload"] = map[string]any{"presignedUrl": saveTo.PresignedURL}
	}
	var r struct {
		RecordingID string `json:"recordingId"`
	}
	timeout := 30 * time.Second
	if kind == "stopRecordingSession" {
		timeout = 180 * time.Second
	}
	if err := c.call(ctx, kind, params, &r, timeout); err != nil {
		return "", err
	}
	filename := "video_recording.mp4"
	if kind == "stopRecordingSession" {
		stopped := r.RecordingID
		if stopped == "" {
			stopped = id
		}
		if !(id == "" && stopped == "legacy:recording") {
			if err := validateRecordingID(stopped); err != nil {
				return "", err
			}
			filename = "recording_" + stopped + ".mp4"
		}
	}
	u := c.http.URL + "/files?path=" + url.QueryEscape("/data/local/tmp/recordings/"+filename)
	if saveTo != nil && saveTo.LocalPath != "" {
		if err := c.http.Download(ctx, u, saveTo.LocalPath); err != nil {
			return "", err
		}
	}
	return u, nil
}
func (c *Client) StartAppLogCapture(ctx context.Context, bundleID string, persist *PersistOption) error {
	if bundleID == "" {
		return errors.New("bundleId must be a non-empty string")
	}
	params, err := persist.Fields()
	if err != nil {
		return err
	}
	params["bundleId"] = bundleID
	return c.call(ctx, "startAppLogCapture", params, nil, 0)
}
func (c *Client) StopAppLogCapture(ctx context.Context) error {
	return c.call(ctx, "stopAppLogCapture", nil, nil, 0)
}
func (c *Client) StartEventCapture(ctx context.Context, persist *PersistOption) error {
	params, err := persist.Fields()
	if err != nil {
		return err
	}
	return c.call(ctx, "startEventCapture", params, nil, 0)
}
func (c *Client) StopEventCapture(ctx context.Context) error {
	return c.call(ctx, "stopEventCapture", nil, nil, 0)
}
func (c *Client) StreamAppLogCapture(ctx context.Context) *session.Stream[session.LogLine] {
	return session.StartStream[session.LogLine](ctx, c.http.HTTPClient, c.http.URL+"/session/applogs/events", c.http.Token)
}
func (c *Client) StreamEventCapture(ctx context.Context) *session.Stream[session.Event] {
	return session.StartStream[session.Event](ctx, c.http.HTTPClient, c.http.URL+"/session/events/events", c.http.Token)
}
