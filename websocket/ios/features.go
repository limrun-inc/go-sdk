package ios

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/limrun-inc/go-sdk/session"
)

type PersistOption = session.PersistOption
type ScreenshotOptions struct {
	Format string `json:"format,omitempty"`
	Size   string `json:"size,omitempty"`
}
type TypeTextOptions struct{ RequireFocus *bool }
type TapElementOptions struct {
	Activate     string
	ScrollSearch bool
	Timeout      time.Duration
}

func (c *Client) ElementTreeRaw(ctx context.Context, point *AccessibilityPoint) (string, error) {
	return c.ElementTree(ctx, point)
}

var absentSelector = regexp.MustCompile(`matched nothing in the accessibility tree|was not found on screen after scrolling|Accessibility element not found for selector`)

func (c *Client) tapElementWithOptions(ctx context.Context, selector AccessibilitySelector, opts TapElementOptions) (*TapElementResult, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	params := map[string]any{"selector": selector}
	if opts.Activate != "" {
		params["activate"] = opts.Activate
	}
	attempt := func() (*TapElementResult, error) {
		var r TapElementResult
		err := c.call(ctx, "tapElement", params, &r, timeout)
		return &r, err
	}
	r, last := attempt()
	if last == nil || !opts.ScrollSearch || !absentSelector.MatchString(last.Error()) {
		return r, last
	}
	info, err := c.DeviceInfo(ctx)
	if err != nil {
		return nil, err
	}
	height := info.ScreenHeight
	display := DuoDisplay("")
	if info.Model == "iPhone Duo" {
		state, err := c.GetFoldState(ctx)
		if err != nil {
			return nil, err
		}
		var active []FoldDisplay
		if state != nil {
			for _, d := range state.Displays {
				if d.Active != nil && *d.Active {
					active = append(active, d)
				}
			}
		}
		if len(active) != 1 {
			return nil, errors.New("scroll search requires exactly one active Duo display; retry after the fold settles")
		}
		d := active[0]
		display = d.ID
		height = d.Height / d.Scale
		if d.Orientation == 3 || d.Orientation == 4 {
			height = d.Width / d.Scale
		}
	}
	for _, phase := range []struct {
		direction ScrollDirection
		count     int
	}{{ScrollDown, 3}, {ScrollUp, 6}} {
		for page := 0; page < phase.count; page++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if err := c.Scroll(ctx, phase.direction, math.Round(height*.6), &ScrollOptions{Display: display}); err != nil {
				return nil, err
			}
			t := time.NewTimer(300 * time.Millisecond)
			select {
			case <-ctx.Done():
				t.Stop()
				return nil, ctx.Err()
			case <-t.C:
			}
			r, last = attempt()
			if last == nil || !absentSelector.MatchString(last.Error()) {
				return r, last
			}
		}
	}
	return nil, fmt.Errorf("%w (after client-side scroll search)", last)
}

type DuoDisplay string

const (
	DuoOuter DuoDisplay = "outer"
	DuoInner DuoDisplay = "inner"
)

type DuoOrientation string

const (
	DuoPortrait           DuoOrientation = "portrait"
	DuoPortraitUpsideDown DuoOrientation = "pud"
	DuoLandscapeLeft      DuoOrientation = "landscape-left"
	DuoLandscapeRight     DuoOrientation = "landscape-right"
)

type FoldDisplay struct {
	ID          DuoDisplay `json:"id"`
	ScreenID    int        `json:"screenId"`
	TrackID     string     `json:"trackId"`
	Width       float64    `json:"width"`
	Height      float64    `json:"height"`
	Scale       float64    `json:"scale"`
	Orientation int        `json:"orientation"`
	Active      *bool      `json:"active,omitempty"`
}
type FoldState struct {
	Orientation     DuoOrientation `json:"orientation"`
	AngleDegrees    float64        `json:"angleDegrees"`
	MinAngleDegrees float64        `json:"minAngleDegrees"`
	MaxAngleDegrees float64        `json:"maxAngleDegrees"`
	Displays        []FoldDisplay  `json:"displays"`
}

func (c *Client) fold(ctx context.Context, kind string, params any) (*FoldState, error) {
	var r struct {
		State *FoldState `json:"state"`
	}
	err := c.call(ctx, kind, params, &r, 0)
	return r.State, err
}
func (c *Client) GetFoldState(ctx context.Context) (*FoldState, error) {
	return c.fold(ctx, "getFoldState", nil)
}
func (c *Client) SetDuoOrientation(ctx context.Context, o DuoOrientation) (*FoldState, error) {
	switch o {
	case DuoPortrait, DuoPortraitUpsideDown, DuoLandscapeLeft, DuoLandscapeRight:
	default:
		return nil, errors.New("invalid iPhone Duo orientation")
	}
	return c.fold(ctx, "setDuoOrientation", map[string]any{"orientation": o})
}
func (c *Client) SetHingeAngle(ctx context.Context, angle float64) (*FoldState, error) {
	if math.IsNaN(angle) || math.IsInf(angle, 0) || angle < 0 || angle > 180 {
		return nil, errors.New("hinge angle must be between 0 and 180 degrees")
	}
	return c.fold(ctx, "setHingeAngle", map[string]any{"angleDegrees": angle})
}
func (c *Client) ScreenshotDisplay(ctx context.Context, display DuoDisplay, opts ...ScreenshotOptions) (*ScreenshotData, error) {
	params := map[string]any{"display": display}
	if len(opts) > 0 {
		if opts[0].Format != "" {
			params["format"] = opts[0].Format
		}
		if opts[0].Size != "" {
			params["size"] = opts[0].Size
		}
	}
	var r ScreenshotData
	err := c.call(ctx, "screenshotDisplay", params, &r, 0)
	return &r, err
}
func (c *Client) TapDisplay(ctx context.Context, display DuoDisplay, x, y float64) error {
	return c.call(ctx, "tapDisplay", map[string]any{"display": display, "x": x, "y": y}, nil, 0)
}

type KeychainRestoreResult struct {
	DurationMs float64 `json:"durationMs"`
}

func (c *Client) SaveKeychain(ctx context.Context, url string) error {
	return c.call(ctx, "exportKeychain", map[string]any{"url": url}, nil, 120*time.Second)
}
func (c *Client) RestoreKeychain(ctx context.Context, url string) (*KeychainRestoreResult, error) {
	var r KeychainRestoreResult
	err := c.call(ctx, "importKeychain", map[string]any{"url": url}, &r, 120*time.Second)
	return &r, err
}

type PlayOnMicrophoneResult struct {
	DurationMs float64 `json:"durationMs"`
	Once       bool    `json:"once"`
}
type MicrophoneStatus struct {
	Source   string `json:"source"`
	FilePath string `json:"filePath,omitempty"`
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
func (c *Client) StopMicrophonePlayback(ctx context.Context) error {
	return c.call(ctx, "stopMicrophonePlayback", nil, nil, 0)
}
func (c *Client) MicrophoneStatus(ctx context.Context) (*MicrophoneStatus, error) {
	r := struct {
		Status MicrophoneStatus `json:"status"`
	}{Status: MicrophoneStatus{Source: "silence"}}
	err := c.call(ctx, "microphoneStatus", nil, &r, 0)
	return &r.Status, err
}
func recordingFields(opts *RecordingOptions) (map[string]any, error) {
	if opts == nil {
		return map[string]any{}, nil
	}
	p, err := opts.Persist.Fields()
	if err != nil {
		return nil, err
	}
	if opts.Quality != 0 {
		if opts.Quality < 5 || opts.Quality > 10 {
			return nil, errors.New("quality must be one of: 5, 6, 7, 8, 9, 10")
		}
		p["quality"] = opts.Quality
	}
	if opts.Display != "" {
		p["display"] = opts.Display
	}
	if opts.TouchIndicators != nil {
		p["touchIndicators"] = *opts.TouchIndicators
	}
	return p, nil
}

var recordingIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func validateRecordingID(id string) error {
	if !recordingIDPattern.MatchString(id) {
		return errors.New("recordingId must be 1–64 lowercase letters, digits, underscores or hyphens, start with a letter or digit")
	}
	return nil
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
	if err = c.call(ctx, "startVideoRecordingSession", params, &r, 0); err != nil {
		return "", err
	}
	return r.RecordingID, validateRecordingID(r.RecordingID)
}
func (c *Client) StopRecordingSession(ctx context.Context, id string, saveTo *SaveRecordingTo) (string, error) {
	params := map[string]any{}
	if id != "" {
		if err := validateRecordingID(id); err != nil {
			return "", err
		}
		params["recordingId"] = id
	}
	if saveTo != nil && saveTo.PresignedURL != "" {
		params["upload"] = recordingUpload{saveTo.PresignedURL}
	}
	var r struct {
		RecordingID string `json:"recordingId"`
	}
	if err := c.call(ctx, "stopVideoRecordingSession", params, &r, 180*time.Second); err != nil {
		return "", err
	}
	stopped := r.RecordingID
	if stopped == "" {
		stopped = id
	}
	filename := "recording.mp4"
	if !(id == "" && stopped == "legacy:recording") {
		if err := validateRecordingID(stopped); err != nil {
			return "", err
		}
		filename = "recording_" + stopped + ".mp4"
	}
	u, err := c.fileURL("files", "name", filename, nil)
	if err != nil {
		return "", err
	}
	if saveTo != nil && saveTo.LocalPath != "" {
		if err = c.downloadFile(ctx, u, saveTo.LocalPath); err != nil {
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
	return session.StartStream[session.LogLine](ctx, c.httpClient, c.apiURL+"/session/applogs/events", c.token)
}
func (c *Client) StreamEventCapture(ctx context.Context) *session.Stream[session.Event] {
	return session.StartStream[session.Event](ctx, c.httpClient, c.apiURL+"/session/events/events", c.token)
}
