// Package session contains persisted capture metadata shared by iOS, Android and macOS.
package session

import "errors"

type PersistOption struct{ TTLSeconds *int64 }

func (p *PersistOption) Fields() (map[string]any, error) {
	m := map[string]any{}
	if p == nil {
		return m, nil
	}
	m["persist"] = true
	if p.TTLSeconds != nil {
		if *p.TTLSeconds <= 0 {
			return nil, errors.New("ttlSeconds must be a positive integer")
		}
		m["ttlSeconds"] = *p.TTLSeconds
	}
	return m, nil
}

type ArtifactKind string

const (
	Recording  ArtifactKind = "recording"
	AppLog     ArtifactKind = "appLog"
	EventLog   ArtifactKind = "eventLog"
	NetworkLog ArtifactKind = "networkLog"
	// Performance holds a macOS instance's app performance samples, one PerformanceSample per line.
	Performance ArtifactKind = "performance"
)

type Artifact struct {
	ID          string       `json:"id"`
	Kind        ArtifactKind `json:"kind"`
	BundleID    string       `json:"bundleId,omitempty"`
	TunnelID    string       `json:"tunnelId,omitempty"`
	Selectors   []string     `json:"selectors,omitempty"`
	StartedAt   string       `json:"startedAt,omitempty"`
	StoppedAt   string       `json:"stoppedAt,omitempty"`
	ExpiresAt   string       `json:"expiresAt,omitempty"`
	SizeBytes   *int64       `json:"sizeBytes,omitempty"`
	DownloadURL string       `json:"downloadUrl"`
}
type LogLine struct {
	TS   float64 `json:"ts"`
	Line string  `json:"line"`
}
type Event struct {
	TS         float64  `json:"ts"`
	Type       string   `json:"type"`
	X          *float64 `json:"x,omitempty"`
	Y          *float64 `json:"y,omitempty"`
	ToX        *float64 `json:"toX,omitempty"`
	ToY        *float64 `json:"toY,omitempty"`
	DurationMs *float64 `json:"durationMs,omitempty"`
	Count      *int64   `json:"count,omitempty"`
	Summary    string   `json:"summary,omitempty"`
}

// PerformanceThread is one thread's share of an app's CPU in a PerformanceSample.
type PerformanceThread struct {
	// Name is the thread's name, "main" for the main thread, or empty.
	Name string `json:"name"`
	// CPUPercent is the share of one core; 100 is one fully busy core.
	CPUPercent float64 `json:"cpuPercent"`
}

// PerformanceSample is one running app's resource use at one moment. Rates cover the second
// since the previous sample of the same process, so a process's first sample has none.
type PerformanceSample struct {
	// TS is epoch milliseconds, the same clock as LogLine.
	TS       float64 `json:"ts"`
	BundleID string  `json:"bundleId"`
	// PID changes when the app relaunches.
	PID int64 `json:"pid"`
	// StartedAt is the process start in epoch milliseconds.
	StartedAt float64 `json:"startedAt"`
	// TestRunner is set for XCTest runners.
	TestRunner bool `json:"testRunner,omitempty"`
	// CPUPercent counts one fully busy core as 100.
	CPUPercent              *float64            `json:"cpuPercent,omitempty"`
	FootprintBytes          int64               `json:"footprintBytes"`
	PeakFootprintBytes      int64               `json:"peakFootprintBytes"`
	ThreadCount             int64               `json:"threadCount"`
	DiskReadBytesPerSecond  *float64            `json:"diskReadBytesPerSecond,omitempty"`
	DiskWriteBytesPerSecond *float64            `json:"diskWriteBytesPerSecond,omitempty"`
	TopThreads              []PerformanceThread `json:"topThreads,omitempty"`
}

type StreamLostError struct{ Cause error }

func (e *StreamLostError) Error() string {
	return "session stream kept failing for 300s without delivering events: " + e.Cause.Error()
}
func (e *StreamLostError) Unwrap() error { return e.Cause }
