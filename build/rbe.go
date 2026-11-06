package build

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/limrun-inc/go-sdk/internal/direct"
)

var transientPattern = regexp.MustCompile(`(?i)failed: (?:502|503|504)\b|\bEOF\b|ECONNRESET|ECONNREFUSED|socket hang up|fetch failed|connection reset by peer|connection refused`)

func IsTransientError(err error) bool { return err != nil && transientPattern.MatchString(err.Error()) }
func DefaultSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type RetryOptions struct {
	Attempts int
	RetryOn  func(error) bool
	Sleep    func(context.Context, time.Duration) error
	Log      func(string)
}

func RetryTransient[T any](ctx context.Context, fn func() (T, error), opts *RetryOptions) (T, error) {
	o := RetryOptions{Attempts: 5, RetryOn: IsTransientError, Sleep: DefaultSleep}
	if opts != nil {
		if opts.Attempts != 0 {
			o.Attempts = opts.Attempts
		}
		if opts.RetryOn != nil {
			o.RetryOn = opts.RetryOn
		}
		if opts.Sleep != nil {
			o.Sleep = opts.Sleep
		}
		o.Log = opts.Log
	}
	var result T
	var err error
	for attempt := 1; attempt <= o.Attempts; attempt++ {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		result, err = fn()
		if err == nil || !o.RetryOn(err) {
			return result, err
		}
		if attempt < o.Attempts {
			if o.Log != nil {
				o.Log("Retrying after error (" + strings.TrimSpace(err.Error()) + ")...")
			}
			if e := o.Sleep(ctx, 2*time.Second*time.Duration(attempt)); e != nil {
				return result, e
			}
		}
	}
	return result, err
}

type WaitForRbeOptions struct {
	MaxAttempts int
	Sleep       func(context.Context, time.Duration) error
}

func WaitForRbeRunning(ctx context.Context, client interface {
	GetRbe(context.Context) (*RbeStatus, error)
}, initial RbeStatus, opts *WaitForRbeOptions) (*RbeStatus, error) {
	attempts := 15
	sleep := DefaultSleep
	if opts != nil {
		if opts.MaxAttempts != 0 {
			attempts = opts.MaxAttempts
		}
		if opts.Sleep != nil {
			sleep = opts.Sleep
		}
	}
	status := &initial
	for i := 0; status.State == "starting" && i < attempts; i++ {
		if err := sleep(ctx, 2*time.Second); err != nil {
			return nil, err
		}
		var err error
		status, err = RetryTransient(ctx, func() (*RbeStatus, error) { return client.GetRbe(ctx) }, &RetryOptions{Sleep: sleep})
		if err != nil {
			return nil, err
		}
	}
	if status.State != "running" || status.FrontendPort == nil || *status.FrontendPort == 0 || status.XcodeVersion == nil || *status.XcodeVersion == "" {
		message := "state is " + status.State
		if status.Error != nil {
			message = *status.Error
		}
		return nil, fmt.Errorf("remote-execution stack failed to start: %s", message)
	}
	return status, nil
}
func (c *XcodeClient) UploadLatestRbeBuild(ctx context.Context, opts BuildUpload) (*RbeUploadResult, error) {
	asset, signed, err := NormalizeAssetUpload(opts)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if asset != nil {
		body["assetUpload"] = asset
	} else {
		body["signedUploadUrl"] = *signed
	}
	return RetryTransient(ctx, func() (*RbeUploadResult, error) {
		var r RbeUploadResult
		err := c.http.JSON(ctx, http.MethodPost, "rbe/upload", body, &r)
		var he *direct.HTTPError
		if errors.As(err, &he) && he.Status == 404 {
			return nil, errors.New("POST /rbe/upload returned 404: the instance may no longer exist, or its limbuild predates RBE upload support. Recreate the instance and retry")
		}
		return &r, err
	}, &RetryOptions{RetryOn: func(err error) bool {
		var he *direct.HTTPError
		return IsTransientError(err) || (errors.As(err, &he) && he.Status == 400 && strings.Contains(strings.ToLower(he.Body), "no successful rbe app build"))
	}})
}
