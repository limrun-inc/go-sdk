package limrun

import (
	"context"
	"net/http"
	"net/url"
	"slices"

	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
)

// MacosSnapshotConfig is a disk snapshot of a macOS instance, restored after the instance is
// assigned and saved at termination. It keeps the files the guest changed since the instance
// started: the user's home, installed apps and tools. It restores only onto an instance with the
// same macOS build. It has the same shape as a Gradle snapshot configuration.
type MacosSnapshotConfig = GradleSnapshotConfig

// MacosSnapshotConfigState is the snapshot configuration a macOS instance reports back.
type MacosSnapshotConfigState = GradleSnapshotConfigState

// MacosInstanceSnapshot is the disk snapshot configuration of a macOS instance and the state of
// its restore and save. The phases are the Gradle snapshot's.
type MacosInstanceSnapshot = GradleInstanceSnapshot

// MacosInstanceStatusSnapshot summarizes a macOS instance's disk snapshot on its own record.
type MacosInstanceStatusSnapshot = GradleInstanceStatusSnapshot

type (
	MacosSnapshotRestoreStatus = GradleSnapshotRestoreStatus
	MacosSnapshotSaveStatus    = GradleSnapshotSaveStatus
	MacosSnapshotSkippedKey    = GradleSnapshotSkippedKey
	MacosSnapshotFollowOptions = GradleSnapshotFollowOptions
	MacosSnapshotFollowResult  = GradleSnapshotFollowResult
	MacosSnapshotFollowTarget  = GradleSnapshotFollowTarget
)

// GetSnapshot returns the current disk snapshot configuration and status, including a save still
// running at termination.
func (r *MacosInstanceService) GetSnapshot(ctx context.Context, id string, opts ...option.RequestOption) (res *MacosInstanceSnapshot, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, "v1/macos_instances/"+url.PathEscape(id)+"/snapshot", nil, &res, slices.Concat(r.Options, opts)...)
	return
}

// BindSnapshotKey sets the key the instance saves under at termination. It is set once: binding
// the same key again is accepted, a different one is rejected. It works on an instance created
// without a snapshot.
func (r *MacosInstanceService) BindSnapshotKey(ctx context.Context, id, key string, opts ...option.RequestOption) (res *MacosInstanceSnapshot, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPut, "v1/macos_instances/"+url.PathEscape(id)+"/snapshot", map[string]string{"key": key}, &res, slices.Concat(r.Options, opts)...)
	return
}

// FollowSnapshot follows the disk snapshot restore or save until it reaches a terminal phase,
// the instance disappears, or the wait times out. It fails with a [*GradleSnapshotGoneError] or a
// [*GradleSnapshotTimeoutError], the errors every disk snapshot follower shares.
func (r *MacosInstanceService) FollowSnapshot(ctx context.Context, id string, options MacosSnapshotFollowOptions, opts ...option.RequestOption) (*MacosSnapshotFollowResult, error) {
	return followInstanceSnapshot(ctx, "macos_instances", id, options, slices.Concat(r.Options, opts))
}

// FollowMacosSnapshot follows a macOS instance's disk snapshot without a client.
func FollowMacosSnapshot(ctx context.Context, target MacosSnapshotFollowTarget, options MacosSnapshotFollowOptions) (*MacosSnapshotFollowResult, error) {
	service := NewMacosInstanceService(option.WithBaseURL(target.BaseURL), option.WithAPIKey(target.APIKey))
	return service.FollowSnapshot(ctx, target.InstanceID, options)
}
