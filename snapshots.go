package limrun

import (
	"context"
	"net/http"
	"net/url"
	"slices"

	"github.com/limrun-inc/go-sdk/internal/apiquery"
	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
)

type SnapshotListParams struct {
	Prefix param.Opt[string] `query:"prefix,omitzero"`
	Limit  param.Opt[int64]  `query:"limit,omitzero"`
	Offset param.Opt[int64]  `query:"offset,omitzero"`
}

func (p SnapshotListParams) URLQuery() (url.Values, error) {
	return apiquery.MarshalWithSettings(p, apiquery.QuerySettings{})
}

type SnapshotList[T any] struct {
	Snapshots  []T    `json:"snapshots"`
	NextOffset *int64 `json:"nextOffset,omitempty"`
}
type XcodeSnapshot struct {
	Key              string   `json:"key"`
	DirectoryKey     string   `json:"directoryKey"`
	RootName         string   `json:"rootName"`
	Paths            []string `json:"paths,omitempty"`
	XcodeVersionKey  string   `json:"xcodeVersionKey"`
	ArchiveFormat    string   `json:"archiveFormat"`
	ArchiveSizeBytes int64    `json:"archiveSizeBytes"`
	SHA256           string   `json:"sha256"`
	PublishedAt      string   `json:"publishedAt"`
	SourceInstanceID string   `json:"sourceInstanceId,omitempty"`
}
type IosSnapshot struct {
	Key              string `json:"key"`
	Saved            bool   `json:"saved"`
	CreatedAt        string `json:"createdAt"`
	LastUsedAt       string `json:"lastUsedAt"`
	ArchiveSizeBytes *int64 `json:"archiveSizeBytes,omitempty"`
	PublishedAt      string `json:"publishedAt,omitempty"`
	IOSVersion       string `json:"iosVersion,omitempty"`
}
type SnapshotService[T any] struct {
	Options  []option.RequestOption
	resource string
}
type XcodeSnapshotService = SnapshotService[XcodeSnapshot]
type IosSnapshotService = SnapshotService[IosSnapshot]

func NewXcodeSnapshotService(opts ...option.RequestOption) XcodeSnapshotService {
	return XcodeSnapshotService{Options: opts, resource: "xcode_snapshots"}
}
func NewIosSnapshotService(opts ...option.RequestOption) IosSnapshotService {
	return IosSnapshotService{Options: opts, resource: "ios_snapshots"}
}
func (r *SnapshotService[T]) path(org string) string {
	return "v1/organizations/" + url.PathEscape(org) + "/" + r.resource
}
func (r *SnapshotService[T]) List(ctx context.Context, org string, params SnapshotListParams, opts ...option.RequestOption) (res *SnapshotList[T], err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, r.path(org), params, &res, slices.Concat(r.Options, opts)...)
	return
}
func (r *SnapshotService[T]) Get(ctx context.Context, org, key string, opts ...option.RequestOption) (res *T, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, r.path(org)+"/"+url.PathEscape(key), nil, &res, slices.Concat(r.Options, opts)...)
	return
}
func (r *SnapshotService[T]) Delete(ctx context.Context, org, key string, opts ...option.RequestOption) error {
	return requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, r.path(org)+"/"+url.PathEscape(key), nil, nil, slices.Concat(r.Options, opts, []option.RequestOption{option.WithHeader("Accept", "*/*")})...)
}
