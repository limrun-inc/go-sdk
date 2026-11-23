package limrun

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sync"

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

// RestoreSnapshot restores the disk snapshot a ready instance was created with
// and waits until the restore ends. The platform finds the snapshot, but only a
// client starts the restore; one no client starts in time is skipped and the
// instance runs without it.
func (r *MacosInstanceService) RestoreSnapshot(ctx context.Context, instance MacosInstance, options MacosSnapshotFollowOptions, opts ...option.RequestOption) (*MacosSnapshotFollowResult, error) {
	client, err := r.CreateClient(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Started once the stream is open, so a restore that is over in seconds is
	// still seen. A start that fails ends the follow, since nothing will change.
	var once sync.Once
	var startErr error
	var startDone sync.WaitGroup
	onOpen := options.OnOpen
	options.Side = "restore"
	options.OnOpen = func() {
		once.Do(func() {
			startDone.Add(1)
			go func() {
				defer startDone.Done()
				if _, err := client.StartSnapshotRestore(ctx); err != nil {
					startErr = err
					cancel()
				}
			}()
		})
		if onOpen != nil {
			onOpen()
		}
	}
	result, err := r.FollowSnapshot(ctx, instance.Metadata.ID, options, opts...)
	startDone.Wait()
	if startErr != nil {
		return result, fmt.Errorf("starting the snapshot restore: %w", startErr)
	}
	return result, err
}

// FollowMacosSnapshot follows a macOS instance's disk snapshot without a client.
func FollowMacosSnapshot(ctx context.Context, target MacosSnapshotFollowTarget, options MacosSnapshotFollowOptions) (*MacosSnapshotFollowResult, error) {
	service := NewMacosInstanceService(option.WithBaseURL(target.BaseURL), option.WithAPIKey(target.APIKey))
	return service.FollowSnapshot(ctx, target.InstanceID, options)
}
