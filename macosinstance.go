// Hand-written following the Stainless resource pattern; a future generation
// from the OpenAPI spec reconciles with this file.

package limrun

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/limrun-inc/go-sdk/internal/apijson"
	"github.com/limrun-inc/go-sdk/internal/apiquery"
	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/pagination"
	"github.com/limrun-inc/go-sdk/packages/param"
	"github.com/limrun-inc/go-sdk/packages/respjson"
)

// MacosInstanceService contains methods and other services that help with
// interacting with the limrun API.
//
// Note, unlike clients, this service does not read variables from the environment
// automatically. You should not instantiate this service directly, and instead use
// the [NewMacosInstanceService] method instead.
type MacosInstanceService struct {
	Options []option.RequestOption
}

// NewMacosInstanceService generates a new service that applies the given options
// to each request. These options are applied after the parent client's options (if
// there is one), and before any request-specific options.
func NewMacosInstanceService(opts ...option.RequestOption) (r MacosInstanceService) {
	r = MacosInstanceService{}
	r.Options = opts
	return
}

// Create a macOS instance
func (r *MacosInstanceService) New(ctx context.Context, params MacosInstanceNewParams, opts ...option.RequestOption) (res *MacosInstance, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/macos_instances"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, params, &res, opts...)
	return res, err
}

// List macOS instances
func (r *MacosInstanceService) List(ctx context.Context, query MacosInstanceListParams, opts ...option.RequestOption) (res *pagination.Items[MacosInstance], err error) {
	var raw *http.Response
	opts = slices.Concat(r.Options, opts)
	opts = append([]option.RequestOption{option.WithResponseInto(&raw)}, opts...)
	path := "v1/macos_instances"
	cfg, err := requestconfig.NewRequestConfig(ctx, http.MethodGet, path, query, &res, opts...)
	if err != nil {
		return nil, err
	}
	err = cfg.Execute()
	if err != nil {
		return nil, err
	}
	res.SetPageConfig(cfg, raw)
	return res, nil
}

// List macOS instances
func (r *MacosInstanceService) ListAutoPaging(ctx context.Context, query MacosInstanceListParams, opts ...option.RequestOption) *pagination.ItemsAutoPager[MacosInstance] {
	return pagination.NewItemsAutoPager(r.List(ctx, query, opts...))
}

// Delete macOS instance with given name
func (r *MacosInstanceService) Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error) {
	opts = slices.Concat(r.Options, opts)
	opts = append([]option.RequestOption{option.WithHeader("Accept", "*/*")}, opts...)
	if id == "" {
		err = errors.New("missing required id parameter")
		return err
	}
	path := fmt.Sprintf("v1/macos_instances/%s", url.PathEscape(id))
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)
	return err
}

// Get macOS instance with given ID
func (r *MacosInstanceService) Get(ctx context.Context, id string, opts ...option.RequestOption) (res *MacosInstance, err error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		err = errors.New("missing required id parameter")
		return nil, err
	}
	path := fmt.Sprintf("v1/macos_instances/%s", url.PathEscape(id))
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)
	return res, err
}

// Update labels or delegated permissions of a live macOS instance. It requires
// the ETag from a live Get in params.IfMatch.
func (r *MacosInstanceService) Update(ctx context.Context, id string, params MacosInstanceUpdateParams, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts, []option.RequestOption{option.WithHeader("If-Match", params.IfMatch), option.WithHeader("Accept", "*/*")})
	return requestconfig.ExecuteNewRequest(ctx, http.MethodPatch, "v1/macos_instances/"+url.PathEscape(id), params, nil, opts...)
}

type MacosInstance struct {
	Metadata MacosInstanceMetadata `json:"metadata" api:"required"`
	Spec     MacosInstanceSpec     `json:"spec" api:"required"`
	Status   MacosInstanceStatus   `json:"status" api:"required"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Metadata    respjson.Field
		Spec        respjson.Field
		Status      respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r MacosInstance) RawJSON() string { return r.JSON.raw }
func (r *MacosInstance) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MacosInstanceMetadata struct {
	ID             string            `json:"id" api:"required"`
	CreatedAt      time.Time         `json:"createdAt" api:"required" format:"date-time"`
	OrganizationID string            `json:"organizationId" api:"required"`
	DisplayName    string            `json:"displayName"`
	Labels         map[string]string `json:"labels"`
	// Immutable name that works like a folder path. Absent for an instance created
	// before names existed.
	Name string `json:"name"`
	// Time of the controller's termination event. Physical cleanup may follow.
	TerminatedAt time.Time `json:"terminatedAt" format:"date-time"`
	// Billing cutoff for a deletion requested while the region is unavailable.
	// Absent for normal regional deletions.
	TerminatingAt time.Time `json:"terminatingAt" format:"date-time"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		ID             respjson.Field
		CreatedAt      respjson.Field
		OrganizationID respjson.Field
		DisplayName    respjson.Field
		Labels         respjson.Field
		Name           respjson.Field
		TerminatedAt   respjson.Field
		TerminatingAt  respjson.Field
		ExtraFields    map[string]respjson.Field
		raw            string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r MacosInstanceMetadata) RawJSON() string { return r.JSON.raw }
func (r *MacosInstanceMetadata) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MacosInstanceSpec struct {
	// Current effective permissions. Present on live GETs; historical records may omit it.
	Role MacosInstanceSpecRole `json:"role"`

	InactivityTimeout string `json:"inactivityTimeout" api:"required" format:"duration"`
	Region            string `json:"region" api:"required"`
	// Maximum runtime regardless of activity.
	HardTimeout string `json:"hardTimeout" format:"duration"`
	// Disk snapshot configuration, absent for an instance created without one.
	Snapshot MacosSnapshotConfigState `json:"snapshot"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Role respjson.Field

		InactivityTimeout respjson.Field
		Region            respjson.Field
		HardTimeout       respjson.Field
		Snapshot          respjson.Field
		ExtraFields       map[string]respjson.Field
		raw               string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r MacosInstanceSpec) RawJSON() string { return r.JSON.raw }
func (r *MacosInstanceSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MacosInstanceSpecRole struct {
	Permissions []string `json:"permissions"`
	JSON        struct {
		Permissions respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

func (r MacosInstanceSpecRole) RawJSON() string { return r.JSON.raw }
func (r *MacosInstanceSpecRole) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MacosInstanceStatus struct {
	// A signed token that grants only streaming on this instance: it opens the
	// screen-streaming WebSocket and nothing else, so a browser can hold it while
	// `token` stays on your backend. Create, get and list return it to callers with
	// full control of the instance; get and list only while the instance runs.
	StreamToken string `json:"streamToken"`

	// A control token for this instance. Grants runtime access and reads, but cannot
	// create, update, or delete instance records.
	Token string `json:"token"`
	// Any of "unknown", "creating", "assigned", "ready", "terminating", "terminated".
	State  string `json:"state" api:"required"`
	APIURL string `json:"apiUrl"`
	// The screen-streaming WebSocket.
	EndpointWebSocketURL string `json:"endpointWebSocketUrl"`
	ErrorMessage         string `json:"errorMessage"`
	// Machine-readable reason for instance termination. Present when state is
	// "terminating" or "terminated". New values may be added over time, so treat any
	// unrecognized value as "Unknown". Known values:
	//
	//   - "UserRequested": terminated by a delete request to the API.
	//   - "InactivityTimeout": the timeout given in spec.inactivityTimeout elapsed.
	//   - "HardTimeout": the timeout given in spec.hardTimeout elapsed.
	//   - "CapacityTimeout": no instance was free when it was created, and none freed
	//     up before its wait for capacity ended.
	//   - "Unknown": terminated for a cause the platform did not attribute, including
	//     instances that failed to get ready during creation. See errorMessage for
	//     details when available.
	TerminationReason string `json:"terminationReason"`
	// What happened to the instance's disk snapshot. GetSnapshot has the full status.
	Snapshot MacosInstanceStatusSnapshot `json:"snapshot"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		StreamToken respjson.Field

		Token                respjson.Field
		State                respjson.Field
		APIURL               respjson.Field
		EndpointWebSocketURL respjson.Field
		ErrorMessage         respjson.Field
		TerminationReason    respjson.Field
		Snapshot             respjson.Field
		ExtraFields          map[string]respjson.Field
		raw                  string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r MacosInstanceStatus) RawJSON() string { return r.JSON.raw }
func (r *MacosInstanceStatus) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MacosInstanceNewParams struct {

	// If there is another instance with given labels and region, return that one
	// instead of creating a new instance. Reusing it requires control permission on
	// that instance.
	ReuseIfExists param.Opt[bool] `query:"reuseIfExists,omitzero" json:"-"`
	// Return after the instance is ready to connect.
	Wait     param.Opt[bool]                `query:"wait,omitzero" json:"-"`
	Metadata MacosInstanceNewParamsMetadata `json:"metadata,omitzero"`
	Spec     MacosInstanceNewParamsSpec     `json:"spec,omitzero"`
	paramObj
}

func (r MacosInstanceNewParams) MarshalJSON() (data []byte, err error) {
	type shadow MacosInstanceNewParams
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *MacosInstanceNewParams) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// URLQuery serializes [MacosInstanceNewParams]'s query parameters as
// `url.Values`.
func (r MacosInstanceNewParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

type MacosInstanceNewParamsMetadata struct {
	DisplayName param.Opt[string] `json:"displayName,omitzero"`
	// Immutable name that works like a folder path, for example u/u123/preview. A
	// permission can target the name or a folder it lies in. Without one, the
	// instance is named after its ID inside the caller's default folder: the
	// person's home folder for a session or a personal API key, shared for an
	// organization API key.
	Name   param.Opt[string] `json:"name,omitzero"`
	Labels map[string]string `json:"labels,omitzero"`
	paramObj
}

func (r MacosInstanceNewParamsMetadata) MarshalJSON() (data []byte, err error) {
	type shadow MacosInstanceNewParamsMetadata
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *MacosInstanceNewParamsMetadata) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MacosInstanceNewParamsSpec struct {
	// Disk snapshot to restore after assignment and save at termination. Omitted means an
	// ordinary instance that restores and saves nothing.
	Snapshot *MacosSnapshotConfig `json:"snapshot,omitzero"`
	// Inline instance role. Omit for the organization's compatibility default.
	Role MacosInstanceNewParamsSpecRole `json:"role,omitzero"`

	// Maximum runtime regardless of activity. Omit to use the organization default
	// for this platform. Set to "0" to disable the hard timeout.
	HardTimeout param.Opt[string] `json:"hardTimeout,omitzero" format:"duration"`
	// After how many minutes of inactivity should the instance be terminated. The
	// timer starts once the instance becomes ready. Example values 1m, 10m, 3h.
	// Omitting this value or passing a non-positive duration uses the organization's
	// macOS default.
	InactivityTimeout param.Opt[string] `json:"inactivityTimeout,omitzero" format:"duration"`
	// Where the instance will be created. If not given, the region is decided based on
	// scheduling clues (client IP) and availability.
	//
	// A region is a preference, not a hard pin: the request always overflows to every
	// other available region, ordered by proximity, when the preferred ones are full.
	//
	// Accepted values:
	//
	//   - A specific region name (e.g. "us-east1"). It is tried first, then the
	//     remaining regions in order of proximity to it. Scheduling clues (client IP)
	//     are ignored when a region is given.
	//   - A region group name (e.g. "us", "eu"). Its member regions are tried first in
	//     their listed order, then the remaining regions by proximity to the first
	//     member.
	//   - A pipe-separated, ordered list of regions (e.g. "us-east1|eu-north1"). Those
	//     are tried first in the given order, then the remaining regions by proximity to
	//     the first.
	//
	// Deprecated: a region is only a preference, not a hard pin; use Jurisdiction to
	// constrain where the instance runs.
	Region param.Opt[string] `json:"region,omitzero"`
	// ClientLocation takes precedence over ClientIP regardless of clue order.
	// ClientLocation requires clientLocation with both latitude and longitude. Without
	// an explicit location or non-empty ClientIP, routing uses Cloudflare visitor
	// coordinates, then the connecting client IP.
	Clues []MacosInstanceNewParamsSpecClue `json:"clues,omitzero"`
	// Restricts scheduling to regions in the given jurisdiction. Unlike region, this
	// is a hard constraint: the request never overflows to a region outside the
	// jurisdiction and fails when no region in the jurisdiction has capacity. A region
	// belongs to a jurisdiction when its name starts with the jurisdiction prefix,
	// e.g. "eu-north1" is in "eu". A region preference pointing outside the
	// jurisdiction is ignored.
	//
	// Any of "us", "eu", "as".
	Jurisdiction string `json:"jurisdiction,omitzero"`
	paramObj
}

func (r MacosInstanceNewParamsSpec) MarshalJSON() (data []byte, err error) {
	type shadow MacosInstanceNewParamsSpec
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *MacosInstanceNewParamsSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[MacosInstanceNewParamsSpec](
		"jurisdiction", "us", "eu", "as",
	)
}

type MacosInstanceNewParamsSpecRole struct {
	// Asset permissions delegated by the creator. An empty array grants nothing.
	Permissions []string `json:"permissions,omitzero"`
	paramObj
}

func (r MacosInstanceNewParamsSpecRole) MarshalJSON() ([]byte, error) {
	type shadow MacosInstanceNewParamsSpecRole
	return param.MarshalObject(r, (*shadow)(&r))
}

// The property Kind is required.
type MacosInstanceNewParamsSpecClue struct {
	// Required for ClientLocation. Both coordinates must be finite and within their
	// ranges. This location takes precedence over ClientIP.
	ClientLocation *MacosInstanceNewParamsSpecClueClientLocation `json:"clientLocation,omitzero"`

	// Any of "ClientIP", "ClientLocation".
	Kind     string            `json:"kind,omitzero" api:"required"`
	ClientIP param.Opt[string] `json:"clientIp,omitzero"`
	paramObj
}

func (r MacosInstanceNewParamsSpecClue) MarshalJSON() (data []byte, err error) {
	type shadow MacosInstanceNewParamsSpecClue
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *MacosInstanceNewParamsSpecClue) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[MacosInstanceNewParamsSpecClue](
		"kind", "ClientIP", "ClientLocation",
	)
}

type MacosInstanceNewParamsSpecClueClientLocation struct {
	// Latitude in decimal degrees, from -90 to 90 inclusive. Zero is valid.
	Latitude float64 `json:"latitude" api:"required"`
	// Longitude in decimal degrees, from -180 to 180 inclusive. Zero is valid.
	Longitude float64 `json:"longitude" api:"required"`
	paramObj
}

func (r MacosInstanceNewParamsSpecClueClientLocation) MarshalJSON() ([]byte, error) {
	type shadow MacosInstanceNewParamsSpecClueClientLocation
	return param.MarshalObject(r, (*shadow)(&r))
}

type MacosInstanceListParams struct {
	EndingBefore param.Opt[string] `query:"endingBefore,omitzero" json:"-"`
	// Labels filter to apply to instances to return. Expects a comma-separated list of
	// key=value pairs (e.g., env=prod,region=us-west).
	LabelSelector param.Opt[string] `query:"labelSelector,omitzero" json:"-"`
	// Maximum number of items to be returned. The default is 50 and the most 500; a
	// larger value returns 500.
	Limit         param.Opt[int64]  `query:"limit,omitzero" json:"-"`
	StartingAfter param.Opt[string] `query:"startingAfter,omitzero" json:"-"`
	// State filter to apply to macOS instances to return. Each comma-separated state
	// will be used as part of an OR clause, e.g. "assigned,ready" will return all
	// instances that are either assigned or ready.
	//
	// Valid states: creating, assigned, ready, terminating, terminated, unknown
	State param.Opt[string] `query:"state,omitzero" json:"-"`
	paramObj
}

// URLQuery serializes [MacosInstanceListParams]'s query parameters as
// `url.Values`.
func (r MacosInstanceListParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

type MacosInstanceUpdateParams struct {
	// Required quoted ETag from a live GET. Missing/empty values are rejected;
	// a concurrent change returns HTTP 412. Unconditional updates are not supported.
	IfMatch string `json:"-"`
	// Replaces all customer labels. Use {} to clear them. Omission preserves labels.
	Metadata MacosInstanceUpdateParamsMetadata `json:"metadata,omitzero"`
	Spec     MacosInstanceUpdateParamsSpec     `json:"spec,omitzero"`
	paramObj
}

func (r MacosInstanceUpdateParams) MarshalJSON() ([]byte, error) {
	type shadow MacosInstanceUpdateParams
	return param.MarshalObject(r, (*shadow)(&r))
}

type MacosInstanceUpdateParamsMetadata struct {
	Labels map[string]string `json:"labels,omitzero"`
	paramObj
}

func (r MacosInstanceUpdateParamsMetadata) MarshalJSON() ([]byte, error) {
	type shadow MacosInstanceUpdateParamsMetadata
	return param.MarshalObject(r, (*shadow)(&r))
}

type MacosInstanceUpdateParamsSpec struct {
	Role MacosInstanceUpdateParamsSpecRole `json:"role,omitzero"`
	paramObj
}

func (r MacosInstanceUpdateParamsSpec) MarshalJSON() ([]byte, error) {
	type shadow MacosInstanceUpdateParamsSpec
	return param.MarshalObject(r, (*shadow)(&r))
}

type MacosInstanceUpdateParamsSpecRole struct {
	// Replaces the complete list. Use [] to remove delegated permissions.
	Permissions []string `json:"permissions,omitzero"`
	paramObj
}

func (r MacosInstanceUpdateParamsSpecRole) MarshalJSON() ([]byte, error) {
	type shadow MacosInstanceUpdateParamsSpecRole
	return param.MarshalObject(r, (*shadow)(&r))
}
