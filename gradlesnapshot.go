package limrun

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
)

// GradleSnapshotConfig is a disk snapshot of a Gradle build sandbox, restored after the instance
// is assigned and saved at termination. It keeps the synced project, Gradle's caches and wrapper
// distributions, ccache and Android SDK packages the build installed.
type GradleSnapshotConfig struct {
	// TTLSeconds limits the age of the published archive. The default is 30 days.
	TTLSeconds param.Opt[int64] `json:"ttlSeconds,omitzero"`
	// Save under this organization-scoped key at termination. Defaults to restoring this key too.
	Key param.Opt[string] `json:"key,omitzero"`
	// Restore in order, trying an exact match then the newest literal-prefix match for each entry.
	RestoreKeys []string `json:"restoreKeys,omitzero"`
	paramObj
}

func (r GradleSnapshotConfig) MarshalJSON() ([]byte, error) {
	type shadow GradleSnapshotConfig
	return param.MarshalObject(r, (*shadow)(&r))
}

// GradleSnapshotConfigState is the snapshot configuration an instance reports back.
type GradleSnapshotConfigState struct {
	Key         string   `json:"key,omitempty"`
	RestoreKeys []string `json:"restoreKeys,omitempty"`
}

type GradleSnapshotSkippedKey struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

// GradleSnapshotRestoreStatus is how far a restore got. "disabled" and "skipped" are not
// failures: the instance builds cold, which is perfectly usable. Only "failed" means the
// workspace was supposed to be warm and something broke.
type GradleSnapshotRestoreStatus struct {
	// Any of "disabled", "pending", "downloading", "extracting", "restored", "skipped", "failed".
	Phase   string `json:"phase"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	// Snapshot key the instance restores from.
	MatchedKey string `json:"matchedKey,omitempty"`
	// Any of "exact_hit", "prefix_hit".
	MatchKind   string                     `json:"matchKind,omitempty"`
	SkippedKeys []GradleSnapshotSkippedKey `json:"skippedKeys,omitempty"`
	// Whether the bytes came through the regional accelerator ("tag") or directly from object
	// storage ("direct").
	Source            string `json:"source,omitempty"`
	Bytes             *int64 `json:"bytes,omitempty"`
	UncompressedBytes *int64 `json:"uncompressedBytes,omitempty"`
	StartedAt         string `json:"startedAt,omitempty"`
	FinishedAt        string `json:"finishedAt,omitempty"`
}

// GradleSnapshotSaveStatus is how far the save at termination got.
type GradleSnapshotSaveStatus struct {
	// Any of "disabled", "idle", "requested", "quiescing", "archiving", "uploading",
	// "published", "skipped", "failed", "timed_out".
	Phase   string `json:"phase"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	// Snapshot key the save publishes under.
	Key               string `json:"key,omitempty"`
	Bytes             *int64 `json:"bytes,omitempty"`
	UncompressedBytes *int64 `json:"uncompressedBytes,omitempty"`
	RequestedAt       string `json:"requestedAt,omitempty"`
	StartedAt         string `json:"startedAt,omitempty"`
	FinishedAt        string `json:"finishedAt,omitempty"`
}

type GradleInstanceSnapshot struct {
	Config  *GradleSnapshotConfigState  `json:"config,omitempty"`
	Restore GradleSnapshotRestoreStatus `json:"restore"`
	Save    GradleSnapshotSaveStatus    `json:"save"`
}

func IsGradleRestoreTerminal(phase string) bool {
	switch phase {
	case "disabled", "restored", "skipped", "failed":
		return true
	}
	return false
}

func IsGradleSaveTerminal(phase string) bool {
	switch phase {
	case "disabled", "published", "skipped", "failed", "timed_out":
		return true
	}
	return false
}

func IsGradleSnapshotTerminal(s GradleInstanceSnapshot, side string) bool {
	if side == "save" {
		return IsGradleSaveTerminal(s.Save.Phase)
	}
	return IsGradleRestoreTerminal(s.Restore.Phase)
}

// GetSnapshot returns the current disk snapshot configuration and status, including a save still
// running at termination.
func (r *GradleInstanceService) GetSnapshot(ctx context.Context, id string, opts ...option.RequestOption) (res *GradleInstanceSnapshot, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, "v1/gradle_instances/"+url.PathEscape(id)+"/snapshot", nil, &res, slices.Concat(r.Options, opts)...)
	return
}

// BindSnapshotKey sets the key the instance saves under at termination. It is set once: binding
// the same key again is accepted, a different one is rejected. It works on an instance created
// without a snapshot.
func (r *GradleInstanceService) BindSnapshotKey(ctx context.Context, id, key string, opts ...option.RequestOption) (res *GradleInstanceSnapshot, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPut, "v1/gradle_instances/"+url.PathEscape(id)+"/snapshot", map[string]string{"key": key}, &res, slices.Concat(r.Options, opts)...)
	return
}

type GradleSnapshotFollowOptions struct {
	// "restore" (the default) or "save".
	Side    string
	Timeout time.Duration
	// Called once per distinct state, including the state the stream opens with.
	OnUpdate func(GradleInstanceSnapshot)
	// Called when the stream is open, and again after each reconnect.
	OnOpen func()
}

type GradleSnapshotFollowResult struct {
	Snapshot GradleInstanceSnapshot
	// True if the instance disappeared before reaching a terminal phase.
	Gone bool
}

// GradleSnapshotGoneError means the instance disappeared before reporting its disk snapshot state.
type GradleSnapshotGoneError struct{ InstanceID string }

func (e *GradleSnapshotGoneError) Error() string {
	return "Instance " + e.InstanceID + " was gone before it reported any disk snapshot status"
}

type GradleSnapshotTimeoutError struct {
	Side     string
	Timeout  time.Duration
	Snapshot *GradleInstanceSnapshot
}

func (e *GradleSnapshotTimeoutError) Error() string {
	return fmt.Sprintf("timed out after %.0fs waiting for the disk snapshot %s to finish", e.Timeout.Seconds(), e.Side)
}

// FollowSnapshot follows the disk snapshot restore or save until it reaches a terminal phase,
// the instance disappears, or the wait times out.
func (r *GradleInstanceService) FollowSnapshot(ctx context.Context, id string, options GradleSnapshotFollowOptions, opts ...option.RequestOption) (*GradleSnapshotFollowResult, error) {
	side := options.Side
	if side == "" {
		side = "restore"
	}
	timeout := options.Timeout
	if timeout == 0 {
		// A restore is bounded at fifteen minutes and a save at ten server-side.
		timeout = 20 * time.Minute
		if side == "save" {
			timeout = 12 * time.Minute
		}
	}
	p := phaseStream[GradleInstanceSnapshot]{
		endpoint:  "v1/gradle_instances/" + url.PathEscape(id) + "/snapshot",
		eventName: "snapshot",
		confirmGone: func(ctx context.Context) error {
			_, err := r.GetSnapshot(ctx, id, opts...)
			return err
		},
		transitionKey: func(s GradleInstanceSnapshot) string {
			return s.Restore.Phase + "/" + s.Restore.Reason + "|" + s.Save.Phase + "/" + s.Save.Reason
		},
		isTerminal: func(s GradleInstanceSnapshot) bool { return IsGradleSnapshotTerminal(s, side) },
		onUpdate:   options.OnUpdate,
		onOpen:     options.OnOpen,
	}
	last, gone, err := p.follow(ctx, timeout, slices.Concat(r.Options, opts))
	var timedOut *errPhaseTimeout[GradleInstanceSnapshot]
	switch {
	case errors.As(err, &timedOut):
		return nil, &GradleSnapshotTimeoutError{side, timeout, timedOut.last}
	case err != nil:
		return nil, err
	case gone && last == nil:
		return nil, &GradleSnapshotGoneError{id}
	}
	return &GradleSnapshotFollowResult{Snapshot: *last, Gone: gone}, nil
}

type GradleSnapshotFollowTarget struct{ BaseURL, APIKey, InstanceID string }

// FollowGradleSnapshot follows an instance's disk snapshot without a client.
func FollowGradleSnapshot(ctx context.Context, target GradleSnapshotFollowTarget, options GradleSnapshotFollowOptions) (*GradleSnapshotFollowResult, error) {
	service := NewGradleInstanceService(option.WithBaseURL(target.BaseURL), option.WithAPIKey(target.APIKey))
	return service.FollowSnapshot(ctx, target.InstanceID, options)
}
