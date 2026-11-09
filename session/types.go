// Package session contains persisted capture metadata shared by iOS and Android.
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
type StreamLostError struct{ Cause error }

func (e *StreamLostError) Error() string {
	return "session stream kept failing for 300s without delivering events: " + e.Cause.Error()
}
func (e *StreamLostError) Unwrap() error { return e.Cause }
