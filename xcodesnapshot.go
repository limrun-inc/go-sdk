package limrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/internal/stream"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
)

type XcodeSnapshotConfig struct {
	Key         param.Opt[string] `json:"key,omitzero"`
	RestoreKeys []string          `json:"restoreKeys,omitzero"`
	Paths       []string          `json:"paths,omitzero"`
	paramObj
}

func (r XcodeSnapshotConfig) MarshalJSON() ([]byte, error) {
	type shadow XcodeSnapshotConfig
	return param.MarshalObject(r, (*shadow)(&r))
}

type XcodeCacheConfig = XcodeSnapshotConfig
type XcodeCacheSkippedKey struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}
type XcodeCacheRestoreStatus struct {
	Phase             string                 `json:"phase"`
	Reason            string                 `json:"reason,omitempty"`
	Message           string                 `json:"message,omitempty"`
	MatchedKey        string                 `json:"matchedKey,omitempty"`
	MatchKind         string                 `json:"matchKind,omitempty"`
	SkippedKeys       []XcodeCacheSkippedKey `json:"skippedKeys,omitempty"`
	Paths             []string               `json:"paths,omitempty"`
	Source            string                 `json:"source,omitempty"`
	Bytes             *int64                 `json:"bytes,omitempty"`
	UncompressedBytes *int64                 `json:"uncompressedBytes,omitempty"`
	StartedAt         string                 `json:"startedAt,omitempty"`
	FinishedAt        string                 `json:"finishedAt,omitempty"`
}
type XcodeCacheSaveStatus struct {
	Phase             string   `json:"phase"`
	Reason            string   `json:"reason,omitempty"`
	Message           string   `json:"message,omitempty"`
	CacheKey          string   `json:"cacheKey,omitempty"`
	Paths             []string `json:"paths,omitempty"`
	Bytes             *int64   `json:"bytes,omitempty"`
	UncompressedBytes *int64   `json:"uncompressedBytes,omitempty"`
	StartedAt         string   `json:"startedAt,omitempty"`
	FinishedAt        string   `json:"finishedAt,omitempty"`
}
type XcodeInstanceCache struct {
	Config  *XcodeSnapshotConfig    `json:"config,omitempty"`
	Restore XcodeCacheRestoreStatus `json:"restore"`
	Save    XcodeCacheSaveStatus    `json:"save"`
}
type XcodeSnapshotSaveStatus struct {
	Phase             string   `json:"phase"`
	Reason            string   `json:"reason,omitempty"`
	Message           string   `json:"message,omitempty"`
	SnapshotKey       string   `json:"snapshotKey,omitempty"`
	Paths             []string `json:"paths,omitempty"`
	Bytes             *int64   `json:"bytes,omitempty"`
	UncompressedBytes *int64   `json:"uncompressedBytes,omitempty"`
	StartedAt         string   `json:"startedAt,omitempty"`
	FinishedAt        string   `json:"finishedAt,omitempty"`
}
type XcodeInstanceSnapshot struct {
	Config  *XcodeSnapshotConfig    `json:"config,omitempty"`
	Restore XcodeCacheRestoreStatus `json:"restore"`
	Save    XcodeSnapshotSaveStatus `json:"save"`
}

func SnapshotFromCache(c XcodeInstanceCache) XcodeInstanceSnapshot {
	return XcodeInstanceSnapshot{Config: c.Config, Restore: c.Restore, Save: XcodeSnapshotSaveStatus{Phase: c.Save.Phase, Reason: c.Save.Reason, Message: c.Save.Message, SnapshotKey: c.Save.CacheKey, Paths: c.Save.Paths, Bytes: c.Save.Bytes, UncompressedBytes: c.Save.UncompressedBytes, StartedAt: c.Save.StartedAt, FinishedAt: c.Save.FinishedAt}}
}
func IsRestoreTerminal(phase string) bool {
	switch phase {
	case "disabled", "unsupported", "restored", "skipped", "failed":
		return true
	}
	return false
}
func IsRestoreFailure(phase string) bool { return phase == "failed" }
func IsSaveTerminal(phase string) bool {
	switch phase {
	case "disabled", "published", "skipped", "failed", "timed_out":
		return true
	}
	return false
}
func IsCacheTerminal(c XcodeInstanceCache, side string) bool {
	if side == "save" {
		return IsSaveTerminal(c.Save.Phase)
	}
	return IsRestoreTerminal(c.Restore.Phase)
}
func IsSnapshotTerminal(c XcodeInstanceSnapshot, side string) bool {
	if side == "save" {
		return IsSaveTerminal(c.Save.Phase)
	}
	return IsRestoreTerminal(c.Restore.Phase)
}
func (r *XcodeInstanceService) GetCache(ctx context.Context, id string, opts ...option.RequestOption) (res *XcodeInstanceCache, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, "v1/xcode_instances/"+url.PathEscape(id)+"/cache", nil, &res, slices.Concat(r.Options, opts)...)
	return
}
func (r *XcodeInstanceService) BindCacheKey(ctx context.Context, id, key string, opts ...option.RequestOption) (res *XcodeInstanceCache, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPut, "v1/xcode_instances/"+url.PathEscape(id)+"/cache", map[string]string{"key": key}, &res, slices.Concat(r.Options, opts)...)
	return
}
func (r *XcodeInstanceService) GetSnapshot(ctx context.Context, id string, opts ...option.RequestOption) (*XcodeInstanceSnapshot, error) {
	cache, err := r.GetCache(ctx, id, opts...)
	if err != nil {
		return nil, err
	}
	snapshot := SnapshotFromCache(*cache)
	return &snapshot, nil
}
func (r *XcodeInstanceService) BindSnapshotKey(ctx context.Context, id, key string, opts ...option.RequestOption) (*XcodeInstanceSnapshot, error) {
	cache, err := r.BindCacheKey(ctx, id, key, opts...)
	if err != nil {
		return nil, err
	}
	snapshot := SnapshotFromCache(*cache)
	return &snapshot, nil
}

type XcodeCacheFollowOptions struct {
	Side     string
	Timeout  time.Duration
	OnUpdate func(XcodeInstanceCache)
	OnOpen   func()
}
type XcodeCacheFollowResult struct {
	Cache XcodeInstanceCache
	Gone  bool
}
type XcodeCacheGoneError struct{ InstanceID string }

func (e *XcodeCacheGoneError) Error() string {
	return "Instance " + e.InstanceID + " was gone before it reported any cache status"
}

type XcodeCacheTimeoutError struct {
	Side    string
	Timeout time.Duration
	Cache   *XcodeInstanceCache
}

func (e *XcodeCacheTimeoutError) Error() string {
	return fmt.Sprintf("timed out after %.0fs waiting for the cache %s to finish", e.Timeout.Seconds(), e.Side)
}

var errCacheTerminal = errors.New("cache terminal state")

func (r *XcodeInstanceService) FollowCache(ctx context.Context, id string, options XcodeCacheFollowOptions, opts ...option.RequestOption) (*XcodeCacheFollowResult, error) {
	side := options.Side
	if side == "" {
		side = "restore"
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = 20 * time.Minute
		if side == "save" {
			timeout = 12 * time.Minute
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var last *XcodeInstanceCache
	lastKey := ""
	delay := time.Second
	gone := false
	endpoint := "v1/xcode_instances/" + url.PathEscape(id) + "/cache"
	for {
		var res *http.Response
		requestOpts := slices.Concat(r.Options, opts, []option.RequestOption{option.WithHeader("Accept", "text/event-stream"), option.WithMaxRetries(0)})
		err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, endpoint, nil, &res, requestOpts...)
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			gone = true
		} else if err == nil {
			if options.OnOpen != nil {
				options.OnOpen()
			}
			err = stream.Read(res.Body, func(e stream.Event) error {
				if e.Type != "cache" && e.Type != "gone" {
					return nil
				}
				var state XcodeInstanceCache
				if err := json.Unmarshal([]byte(e.Data), &state); err != nil {
					return fmt.Errorf("cache stream sent unreadable data: %w", err)
				}
				last = &state
				if e.Type == "gone" {
					_, confirmErr := r.GetCache(ctx, id, opts...)
					var httpErr *Error
					if errors.As(confirmErr, &httpErr) && httpErr.StatusCode == 404 {
						gone = true
						return errCacheTerminal
					}
					return nil
				}
				key := state.Restore.Phase + "/" + state.Restore.Reason + "|" + state.Save.Phase + "/" + state.Save.Reason
				if key != lastKey {
					lastKey = key
					if options.OnUpdate != nil {
						options.OnUpdate(state)
					}
				}
				if IsCacheTerminal(state, side) {
					return errCacheTerminal
				}
				return nil
			}, func() {}, func(d time.Duration) { delay = d })
			res.Body.Close()
			if errors.Is(err, errCacheTerminal) && !gone {
				return &XcodeCacheFollowResult{Cache: *last}, nil
			}
			if err != nil && !errors.Is(err, errCacheTerminal) && ctx.Err() == nil {
				var syntax *json.SyntaxError
				if errors.As(err, &syntax) {
					return nil, err
				}
			}
		}
		if gone {
			if last == nil {
				return nil, &XcodeCacheGoneError{id}
			}
			return &XcodeCacheFollowResult{Cache: *last, Gone: true}, nil
		}
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, &XcodeCacheTimeoutError{side, timeout, last}
			}
			return nil, ctx.Err()
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

type XcodeSnapshotFollowOptions struct {
	Side     string
	Timeout  time.Duration
	OnUpdate func(XcodeInstanceSnapshot)
	OnOpen   func()
}
type XcodeSnapshotFollowResult struct {
	Snapshot XcodeInstanceSnapshot
	Gone     bool
}
type XcodeSnapshotGoneError struct{ InstanceID string }

func (e *XcodeSnapshotGoneError) Error() string {
	return "Instance " + e.InstanceID + " was gone before it reported any disk snapshot status"
}

type XcodeSnapshotTimeoutError struct {
	Side     string
	Timeout  time.Duration
	Snapshot *XcodeInstanceSnapshot
}

func (e *XcodeSnapshotTimeoutError) Error() string {
	return fmt.Sprintf("timed out after %.0fs waiting for the disk snapshot %s to finish", e.Timeout.Seconds(), e.Side)
}
func (r *XcodeInstanceService) FollowSnapshot(ctx context.Context, id string, o XcodeSnapshotFollowOptions, opts ...option.RequestOption) (*XcodeSnapshotFollowResult, error) {
	options := XcodeCacheFollowOptions{Side: o.Side, Timeout: o.Timeout, OnOpen: o.OnOpen}
	if o.OnUpdate != nil {
		options.OnUpdate = func(c XcodeInstanceCache) { o.OnUpdate(SnapshotFromCache(c)) }
	}
	res, err := r.FollowCache(ctx, id, options, opts...)
	var gone *XcodeCacheGoneError
	var timeout *XcodeCacheTimeoutError
	if errors.As(err, &gone) {
		return nil, &XcodeSnapshotGoneError{gone.InstanceID}
	}
	if errors.As(err, &timeout) {
		var snapshot *XcodeInstanceSnapshot
		if timeout.Cache != nil {
			value := SnapshotFromCache(*timeout.Cache)
			snapshot = &value
		}
		return nil, &XcodeSnapshotTimeoutError{timeout.Side, timeout.Timeout, snapshot}
	}
	if err != nil {
		return nil, err
	}
	return &XcodeSnapshotFollowResult{SnapshotFromCache(res.Cache), res.Gone}, nil
}

type XcodeCacheFollowTarget struct{ BaseURL, APIKey, InstanceID string }
type XcodeSnapshotFollowTarget = XcodeCacheFollowTarget
type XcodeSnapshotRestoreStatus = XcodeCacheRestoreStatus
type XcodeSnapshotSkippedKey = XcodeCacheSkippedKey

func FollowXcodeCache(ctx context.Context, target XcodeCacheFollowTarget, options XcodeCacheFollowOptions) (*XcodeCacheFollowResult, error) {
	service := NewXcodeInstanceService(option.WithBaseURL(target.BaseURL), option.WithAPIKey(target.APIKey))
	return service.FollowCache(ctx, target.InstanceID, options)
}
func FollowXcodeSnapshot(ctx context.Context, target XcodeSnapshotFollowTarget, options XcodeSnapshotFollowOptions) (*XcodeSnapshotFollowResult, error) {
	service := NewXcodeInstanceService(option.WithBaseURL(target.BaseURL), option.WithAPIKey(target.APIKey))
	return service.FollowSnapshot(ctx, target.InstanceID, options)
}
