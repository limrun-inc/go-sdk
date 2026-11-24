package ios

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/limrun-inc/go-sdk/internal/direct"
)

// ErrAppProfilingUnavailable reports that the instance answered 404 to a stack
// sample or memory report: its runtime predates them, or the instance no
// longer exists.
var ErrAppProfilingUnavailable = errors.New("this instance cannot profile apps: its runtime predates stack samples and memory reports, or the instance no longer exists")

// SampleStacksOptions selects the app SampleStacks samples, and for how long.
type SampleStacksOptions struct {
	// BundleID defaults to the app launched last, leaving out XCTest runners.
	BundleID string
	// DurationSeconds is 1 to 30. A zero value uses the default (5).
	DurationSeconds int
}

// StackSample is the call stacks of a running app over a few seconds.
type StackSample struct {
	BundleID string `json:"bundleId"`
	// PID is the process that was sampled.
	PID             int `json:"pid"`
	DurationSeconds int `json:"durationSeconds"`
	// Report is the report of the simulator's sample tool, as the tool printed it.
	Report string `json:"report"`
}

// MemoryReportOptions selects the app MemoryReport inspects.
type MemoryReportOptions struct {
	// BundleID defaults to the app launched last, leaving out XCTest runners.
	BundleID string
}

// MemoryReport is the memory of a running app at one moment.
type MemoryReport struct {
	BundleID string `json:"bundleId"`
	// PID is the process that was inspected.
	PID int `json:"pid"`
	// FootprintBytes is the app's memory footprint when the report started.
	FootprintBytes int64 `json:"footprintBytes"`
	// PeakFootprintBytes is the highest footprint of this process since it started.
	PeakFootprintBytes int64 `json:"peakFootprintBytes"`
	// LeakCount is the number of leaked allocations found. It is nil when the
	// tool's totals could not be read.
	LeakCount *int `json:"leakCount"`
	// LeakedBytes is the bytes held by those allocations.
	LeakedBytes *int64 `json:"leakedBytes"`
	// Leaks is the report of the simulator's leaks tool, as the tool printed it.
	Leaks string `json:"leaks"`
	// Heap is the heap by class, from heap -s.
	Heap string `json:"heap"`
	// Regions is the memory regions, from vmmap --summary.
	Regions string `json:"regions"`
}

// SampleStacks samples the call stacks of a running app to show where it
// spends time. The app keeps running. Only apps the caller installed can be
// sampled. A simulator runs on a Mac, so the report shows which code is busy,
// not how long the same work takes on a device.
//
// It returns ErrAppProfilingUnavailable when the instance's runtime predates
// this, and the instance's refusal otherwise: the app is not running, or
// another sample or report is in progress.
func (c *Client) SampleStacks(ctx context.Context, opts *SampleStacksOptions) (*StackSample, error) {
	body := map[string]any{"durationSeconds": 5}
	if opts != nil {
		if opts.DurationSeconds != 0 {
			body["durationSeconds"] = opts.DurationSeconds
		}
		if opts.BundleID != "" {
			body["bundleId"] = opts.BundleID
		}
	}
	seconds := body["durationSeconds"].(int)
	if seconds < 1 || seconds > 30 {
		return nil, errors.New("sampleStacks: DurationSeconds must be between 1 and 30")
	}
	var sample StackSample
	if err := c.profileApp(ctx, "sampleStacks", "stacks", body, seconds+30, &sample); err != nil {
		return nil, err
	}
	return &sample, nil
}

// MemoryReport reports the memory of a running app: its footprint, leaked
// allocations, its heap by class and its memory regions. The app keeps
// running. Only apps the caller installed can be inspected.
//
// It returns ErrAppProfilingUnavailable when the instance's runtime predates
// this, and the instance's refusal otherwise: the app is not running, or
// another sample or report is in progress.
func (c *Client) MemoryReport(ctx context.Context, opts *MemoryReportOptions) (*MemoryReport, error) {
	body := map[string]any{}
	if opts != nil && opts.BundleID != "" {
		body["bundleId"] = opts.BundleID
	}
	var report MemoryReport
	// The instance runs three tools, each with a minute to finish.
	if err := c.profileApp(ctx, "memoryReport", "memory", body, 180, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// profileApp posts to one of the instance's profiling operations. The instance
// ends a tool that overruns serverSeconds; the extra ten seconds let its own
// answer arrive before this side gives up.
func (c *Client) profileApp(ctx context.Context, operation, name string, body map[string]any, serverSeconds int, result any) error {
	u, err := c.apiBaseURL()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(serverSeconds+10)*time.Second)
	defer cancel()
	err = c.httpJSON(ctx, http.MethodPost, u.JoinPath("performance", name).String(), body, result)
	if direct.IsHTTPError(err, http.StatusNotFound) {
		return ErrAppProfilingUnavailable
	}
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
