// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package limrun

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/limrun-inc/go-sdk/session"

	"github.com/limrun-inc/go-sdk/internal/apijson"
	"github.com/limrun-inc/go-sdk/internal/apiquery"
	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/pagination"
	"github.com/limrun-inc/go-sdk/packages/param"
	"github.com/limrun-inc/go-sdk/packages/respjson"
)

// IosInstanceService contains methods and other services that help with
// interacting with the limrun API.
//
// Note, unlike clients, this service does not read variables from the environment
// automatically. You should not instantiate this service directly, and instead use
// the [NewIosInstanceService] method instead.
type IosInstanceService struct {
	Options []option.RequestOption
}

// NewIosInstanceService generates a new service that applies the given options to
// each request. These options are applied after the parent client's options (if
// there is one), and before any request-specific options.
func NewIosInstanceService(opts ...option.RequestOption) (r IosInstanceService) {
	r = IosInstanceService{}
	r.Options = opts
	return
}

// Create an iOS instance
func (r *IosInstanceService) New(ctx context.Context, params IosInstanceNewParams, opts ...option.RequestOption) (res *IosInstance, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/ios_instances"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, params, &res, opts...)
	return res, err
}

// List iOS instances
func (r *IosInstanceService) List(ctx context.Context, query IosInstanceListParams, opts ...option.RequestOption) (res *pagination.Items[IosInstance], err error) {
	var raw *http.Response
	opts = slices.Concat(r.Options, opts)
	opts = append([]option.RequestOption{option.WithResponseInto(&raw)}, opts...)
	path := "v1/ios_instances"
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

// List iOS instances
func (r *IosInstanceService) ListAutoPaging(ctx context.Context, query IosInstanceListParams, opts ...option.RequestOption) *pagination.ItemsAutoPager[IosInstance] {
	return pagination.NewItemsAutoPager(r.List(ctx, query, opts...))
}

// Delete iOS instance with given name
func (r *IosInstanceService) Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error) {
	opts = slices.Concat(r.Options, opts)
	opts = append([]option.RequestOption{option.WithHeader("Accept", "*/*")}, opts...)
	if id == "" {
		err = errors.New("missing required id parameter")
		return err
	}
	path := fmt.Sprintf("v1/ios_instances/%s", url.PathEscape(id))
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)
	return err
}

// Get iOS instance with given ID
func (r *IosInstanceService) Get(ctx context.Context, id string, opts ...option.RequestOption) (res *IosInstance, err error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		err = errors.New("missing required id parameter")
		return nil, err
	}
	path := fmt.Sprintf("v1/ios_instances/%s", url.PathEscape(id))
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)
	return res, err
}

type IosInstance struct {
	Metadata IosInstanceMetadata `json:"metadata" api:"required"`
	Spec     IosInstanceSpec     `json:"spec" api:"required"`
	Status   IosInstanceStatus   `json:"status" api:"required"`
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
func (r IosInstance) RawJSON() string { return r.JSON.raw }
func (r *IosInstance) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type IosInstanceMetadata struct {
	ID             string            `json:"id" api:"required"`
	CreatedAt      time.Time         `json:"createdAt" api:"required" format:"date-time"`
	OrganizationID string            `json:"organizationId" api:"required"`
	DisplayName    string            `json:"displayName"`
	Labels         map[string]string `json:"labels"`
	TerminatedAt   time.Time         `json:"terminatedAt" format:"date-time"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		ID             respjson.Field
		CreatedAt      respjson.Field
		OrganizationID respjson.Field
		DisplayName    respjson.Field
		Labels         respjson.Field
		TerminatedAt   respjson.Field
		ExtraFields    map[string]respjson.Field
		raw            string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r IosInstanceMetadata) RawJSON() string { return r.JSON.raw }
func (r *IosInstanceMetadata) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type IosInstanceSpec struct {
	// Current effective permissions. Present on live GETs; historical records may omit it.
	Role IosInstanceSpecRole `json:"role"`
	// Names a private tunnel, created in the console (Network) or the API, that
	// serves this instance. Creation fails while no connector (`lim tunnel run`)
	// holds it, and the instance becomes ready only after the tunnel attaches.
	Tunnel string `json:"tunnel"`

	// After how many minutes of inactivity should the instance be terminated. The
	// timer starts once the instance becomes ready. Example values 1m, 10m, 3h.
	// Default is 3m. Providing "0" uses the organization's default inactivity timeout.
	InactivityTimeout string `json:"inactivityTimeout" api:"required" format:"duration"`
	// The region where the instance will be created. If not given, will be decided
	// based on scheduling clues and availability.
	Region string `json:"region" api:"required"`
	// After how many minutes should the instance be terminated. Example values 1m,
	// 10m, 3h. Default is "0" which means no hard timeout.
	HardTimeout string `json:"hardTimeout" format:"duration"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Role   respjson.Field
		Tunnel respjson.Field

		InactivityTimeout respjson.Field
		Region            respjson.Field
		HardTimeout       respjson.Field
		ExtraFields       map[string]respjson.Field
		raw               string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r IosInstanceSpec) RawJSON() string { return r.JSON.raw }
func (r *IosInstanceSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type IosInstanceStatus struct {
	// A signed token that grants only streaming on this instance: it opens the
	// screen-streaming WebSocket and nothing else, so a browser can hold it while
	// `token` stays on your backend. Create, get and list return it to callers with
	// full control of the instance; get and list only while the instance runs.
	StreamToken string `json:"streamToken"`

	Token string `json:"token"`
	// Any of "unknown", "creating", "assigned", "ready", "terminated".
	State                   string                   `json:"state" api:"required"`
	APIURL                  string                   `json:"apiUrl"`
	EndpointWebSocketURL    string                   `json:"endpointWebSocketUrl"`
	ErrorMessage            string                   `json:"errorMessage"`
	McpURL                  string                   `json:"mcpUrl"`
	Sandbox                 IosInstanceStatusSandbox `json:"sandbox"`
	SignedStreamURL         string                   `json:"signedStreamUrl"`
	TargetHTTPPortURLPrefix string                   `json:"targetHttpPortUrlPrefix"`
	// Machine-readable reason the instance was terminated. Always present once state
	// is "terminated", never present before that. New values may be added over time,
	// so treat any unrecognized value as "Unknown". Known values:
	//
	//   - "UserRequested": terminated by a delete request to the API.
	//   - "InactivityTimeout": the timeout given in spec.inactivityTimeout elapsed.
	//   - "HardTimeout": the timeout given in spec.hardTimeout elapsed.
	//   - "Unknown": terminated for a cause the platform did not attribute, including
	//     instances that failed to get ready during creation. See errorMessage for
	//     details when available.
	TerminationReason string `json:"terminationReason"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		StreamToken respjson.Field

		Token                   respjson.Field
		State                   respjson.Field
		APIURL                  respjson.Field
		EndpointWebSocketURL    respjson.Field
		ErrorMessage            respjson.Field
		McpURL                  respjson.Field
		Sandbox                 respjson.Field
		SignedStreamURL         respjson.Field
		TargetHTTPPortURLPrefix respjson.Field
		TerminationReason       respjson.Field
		ExtraFields             map[string]respjson.Field
		raw                     string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r IosInstanceStatus) RawJSON() string { return r.JSON.raw }
func (r *IosInstanceStatus) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type IosInstanceStatusSandbox struct {
	Xcode IosInstanceStatusSandboxXcode `json:"xcode"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Xcode       respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r IosInstanceStatusSandbox) RawJSON() string { return r.JSON.raw }
func (r *IosInstanceStatusSandbox) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type IosInstanceStatusSandboxXcode struct {
	URL string `json:"url"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		URL         respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r IosInstanceStatusSandboxXcode) RawJSON() string { return r.JSON.raw }
func (r *IosInstanceStatusSandboxXcode) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type IosInstanceNewParams struct {

	// If there is another instance with given labels and region, return that one
	// instead of creating a new instance.
	ReuseIfExists param.Opt[bool] `query:"reuseIfExists,omitzero" json:"-"`
	// Return after the instance is ready to connect.
	Wait     param.Opt[bool]              `query:"wait,omitzero" json:"-"`
	Metadata IosInstanceNewParamsMetadata `json:"metadata,omitzero"`
	Spec     IosInstanceNewParamsSpec     `json:"spec,omitzero"`
	paramObj
}

func (r IosInstanceNewParams) MarshalJSON() (data []byte, err error) {
	type shadow IosInstanceNewParams
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *IosInstanceNewParams) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// URLQuery serializes [IosInstanceNewParams]'s query parameters as `url.Values`.
func (r IosInstanceNewParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

type IosInstanceNewParamsMetadata struct {
	DisplayName param.Opt[string] `json:"displayName,omitzero"`
	Labels      map[string]string `json:"labels,omitzero"`
	paramObj
}

func (r IosInstanceNewParamsMetadata) MarshalJSON() (data []byte, err error) {
	type shadow IosInstanceNewParamsMetadata
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *IosInstanceNewParamsMetadata) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type IosInstanceNewParamsSpec struct {
	// Inline instance role. Omit for the organization's compatibility default.
	Role IosInstanceNewParamsSpecRole `json:"role,omitzero"`
	// Names a private tunnel, created in the console (Network) or the API, that
	// serves this instance. Creation fails while no connector (`lim tunnel run`)
	// holds it, and the instance becomes ready only after the tunnel attaches.
	Tunnel param.Opt[string] `json:"tunnel,omitzero"`

	// Keeps this app in the foreground after it is first observed there. This does not
	// launch the app when the simulator starts. Once armed, closing or backgrounding
	// the app causes it to be brought back to the foreground.
	ForceBundleID param.Opt[string] `json:"forceBundleId,omitzero"`
	// After how many minutes should the instance be terminated. Example values 1m,
	// 10m, 3h. Default is "0" which means no hard timeout.
	HardTimeout param.Opt[string] `json:"hardTimeout,omitzero" format:"duration"`
	// After how many minutes of inactivity should the instance be terminated. The
	// timer starts once the instance becomes ready. Example values 1m, 10m, 3h.
	// Default is 3m. Providing "0" uses the organization's default inactivity timeout.
	InactivityTimeout param.Opt[string] `json:"inactivityTimeout,omitzero" format:"duration"`
	// Where the instance will be created. If not given, the region is decided based on
	// scheduling clues (client IP) and availability.
	//
	// A region is a preference, not a hard pin: the request always overflows to every
	// other available region, ordered by proximity, when the preferred ones are full.
	//
	// Accepted values:
	//
	//   - A specific region name (e.g. "us-west1"). It is tried first, then the
	//     remaining regions in order of proximity to it. Scheduling clues (client IP)
	//     are ignored when a region is given.
	//   - A region group name (e.g. "us", "eu"). Its member regions are tried first in
	//     their listed order, then the remaining regions by proximity to the first
	//     member.
	//   - A pipe-separated, ordered list of regions (e.g. "us-east1|us-west1"). Those
	//     are tried first in the given order, then the remaining regions by proximity to
	//     the first.
	Region        param.Opt[string]                      `json:"region,omitzero"`
	Clues         []IosInstanceNewParamsSpecClue         `json:"clues,omitzero"`
	InitialAssets []IosInstanceNewParamsSpecInitialAsset `json:"initialAssets,omitzero"`
	// Restricts scheduling to regions in the given jurisdiction. Unlike region, this
	// is a hard constraint: the request never overflows to a region outside the
	// jurisdiction and fails when no region in the jurisdiction has capacity. A region
	// belongs to a jurisdiction when its name starts with the jurisdiction prefix,
	// e.g. "eu-north1" is in "eu". A region preference pointing outside the
	// jurisdiction is ignored.
	//
	// Any of "us", "eu", "as".
	Jurisdiction string `json:"jurisdiction,omitzero"`
	// The model for the Apple Simulator. Default is iphone.
	//
	// Any of "iphone", "ipad", "watch".
	Model   string                          `json:"model,omitzero"`
	Sandbox IosInstanceNewParamsSpecSandbox `json:"sandbox,omitzero"`
	paramObj
}

func (r IosInstanceNewParamsSpec) MarshalJSON() (data []byte, err error) {
	type shadow IosInstanceNewParamsSpec
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *IosInstanceNewParamsSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[IosInstanceNewParamsSpec](
		"jurisdiction", "us", "eu", "as",
	)
	apijson.RegisterFieldValidator[IosInstanceNewParamsSpec](
		"model", "iphone", "iphone-duo", "ipad", "watch",
	)
}

// The property Kind is required.
type IosInstanceNewParamsSpecClue struct {
	// Required for ClientLocation. Both coordinates must be finite and within their
	// ranges. This location takes precedence over ClientIP.
	ClientLocation *IosInstanceNewParamsSpecClueClientLocation `json:"clientLocation,omitzero"`
	// The major version of iOS, "26" or "27". iphone runs both, iphone-duo runs 27,
	// ipad and watch run 26. Default is 26, or 27 on iphone-duo.
	OSVersion param.Opt[string] `json:"osVersion,omitzero"`

	// Any of "ClientIP".
	Kind     string            `json:"kind,omitzero" api:"required"`
	ClientIP param.Opt[string] `json:"clientIp,omitzero"`
	paramObj
}

func (r IosInstanceNewParamsSpecClue) MarshalJSON() (data []byte, err error) {
	type shadow IosInstanceNewParamsSpecClue
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *IosInstanceNewParamsSpecClue) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[IosInstanceNewParamsSpecClue](
		"kind", "ClientIP", "ClientLocation", "OSVersion",
	)
}

// The properties Kind, Source are required.
type IosInstanceNewParamsSpecInitialAsset struct {

	// Any of "App", "Keychain".
	Kind string `json:"kind,omitzero" api:"required"`
	// Any of "URL", "AssetName", "AssetID".
	Source    string            `json:"source,omitzero" api:"required"`
	AssetID   param.Opt[string] `json:"assetId,omitzero"`
	AssetName param.Opt[string] `json:"assetName,omitzero"`
	// Base64/base64url-encoded 32-byte key used to decrypt Keychain assets. Required
	// when kind is Keychain.
	EncryptionKey param.Opt[string] `json:"encryptionKey,omitzero"`
	URL           param.Opt[string] `json:"url,omitzero"`
	// Launch mode specifies how to launch the app after installation.
	//
	// Any of "ForegroundIfRunning", "RelaunchIfRunning", "FailIfRunning".
	LaunchMode string `json:"launchMode,omitzero"`
	paramObj
}

func (r IosInstanceNewParamsSpecInitialAsset) MarshalJSON() (data []byte, err error) {
	type shadow IosInstanceNewParamsSpecInitialAsset
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *IosInstanceNewParamsSpecInitialAsset) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[IosInstanceNewParamsSpecInitialAsset](
		"kind", "App", "Keychain",
	)
	apijson.RegisterFieldValidator[IosInstanceNewParamsSpecInitialAsset](
		"source", "URL", "AssetName", "AssetID",
	)
	apijson.RegisterFieldValidator[IosInstanceNewParamsSpecInitialAsset](
		"launchMode", "ForegroundIfRunning", "RelaunchIfRunning", "FailIfRunning",
	)
}

type IosInstanceNewParamsSpecSandbox struct {
	Xcode IosInstanceNewParamsSpecSandboxXcode `json:"xcode,omitzero"`
	paramObj
}

func (r IosInstanceNewParamsSpecSandbox) MarshalJSON() (data []byte, err error) {
	type shadow IosInstanceNewParamsSpecSandbox
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *IosInstanceNewParamsSpecSandbox) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type IosInstanceNewParamsSpecSandboxXcode struct {
	Enabled param.Opt[bool] `json:"enabled,omitzero"`
	paramObj
}

func (r IosInstanceNewParamsSpecSandboxXcode) MarshalJSON() (data []byte, err error) {
	type shadow IosInstanceNewParamsSpecSandboxXcode
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *IosInstanceNewParamsSpecSandboxXcode) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type IosInstanceListParams struct {
	EndingBefore param.Opt[string] `query:"endingBefore,omitzero" json:"-"`
	// Labels filter to apply to instances to return. Expects a comma-separated list of
	// key=value pairs (e.g., env=prod,region=us-west).
	LabelSelector param.Opt[string] `query:"labelSelector,omitzero" json:"-"`
	// Maximum number of items to be returned. The default is 50.
	Limit param.Opt[int64] `query:"limit,omitzero" json:"-"`
	// Region where the instance is scheduled on.
	Region        param.Opt[string] `query:"region,omitzero" json:"-"`
	StartingAfter param.Opt[string] `query:"startingAfter,omitzero" json:"-"`
	// State filter to apply to Android instances to return. Each comma-separated state
	// will be used as part of an OR clause, e.g. "assigned,ready" will return all
	// instances that are either assigned or ready.
	//
	// Valid states: creating, assigned, ready, terminated, unknown
	State param.Opt[string] `query:"state,omitzero" json:"-"`
	paramObj
}

// URLQuery serializes [IosInstanceListParams]'s query parameters as `url.Values`.
func (r IosInstanceListParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

type IosInstanceSpecRole struct {
	Permissions []string `json:"permissions"`
	JSON        struct {
		Permissions respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

func (r IosInstanceSpecRole) RawJSON() string                  { return r.JSON.raw }
func (r *IosInstanceSpecRole) UnmarshalJSON(data []byte) error { return apijson.UnmarshalRoot(data, r) }

type IosInstanceUpdateParams struct {
	// Quoted ETag from a live GET. A concurrent change returns HTTP 412.
	IfMatch string `json:"-"`
	// Replaces all customer labels. Use {} to clear them. Omission preserves labels.
	Metadata IosInstanceUpdateParamsMetadata `json:"metadata,omitzero"`
	Spec     IosInstanceUpdateParamsSpec     `json:"spec,omitzero"`
	paramObj
}

func (r IosInstanceUpdateParams) MarshalJSON() ([]byte, error) {
	type shadow IosInstanceUpdateParams
	return param.MarshalObject(r, (*shadow)(&r))
}

type IosInstanceUpdateParamsMetadata struct {
	Labels map[string]string `json:"labels,omitzero"`
	paramObj
}

func (r IosInstanceUpdateParamsMetadata) MarshalJSON() ([]byte, error) {
	type shadow IosInstanceUpdateParamsMetadata
	return param.MarshalObject(r, (*shadow)(&r))
}

type IosInstanceUpdateParamsSpec struct {
	Role IosInstanceUpdateParamsSpecRole `json:"role,omitzero"`
	paramObj
}

func (r IosInstanceUpdateParamsSpec) MarshalJSON() ([]byte, error) {
	type shadow IosInstanceUpdateParamsSpec
	return param.MarshalObject(r, (*shadow)(&r))
}

type IosInstanceUpdateParamsSpecRole struct {
	// Replaces the complete list. Use [] to remove delegated permissions.
	Permissions []string `json:"permissions,omitzero"`
	paramObj
}

func (r IosInstanceUpdateParamsSpecRole) MarshalJSON() ([]byte, error) {
	type shadow IosInstanceUpdateParamsSpecRole
	return param.MarshalObject(r, (*shadow)(&r))
}

type IosInstanceNewParamsSpecRole struct {
	// Asset permissions delegated by the creator. An empty array grants nothing.
	Permissions []string `json:"permissions,omitzero"`
	paramObj
}

func (r IosInstanceNewParamsSpecRole) MarshalJSON() ([]byte, error) {
	type shadow IosInstanceNewParamsSpecRole
	return param.MarshalObject(r, (*shadow)(&r))
}

type IosInstanceNewParamsSpecClueClientLocation struct {
	// Latitude in decimal degrees, from -90 to 90 inclusive. Zero is valid.
	Latitude float64 `json:"latitude,omitzero" api:"required"`
	// Longitude in decimal degrees, from -180 to 180 inclusive. Zero is valid.
	Longitude float64 `json:"longitude,omitzero" api:"required"`
	paramObj
}

func (r IosInstanceNewParamsSpecClueClientLocation) MarshalJSON() ([]byte, error) {
	type shadow IosInstanceNewParamsSpecClueClientLocation
	return param.MarshalObject(r, (*shadow)(&r))
}

func (r *IosInstanceService) Update(ctx context.Context, id string, params IosInstanceUpdateParams, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts, []option.RequestOption{option.WithHeader("If-Match", params.IfMatch), option.WithHeader("Accept", "*/*")})
	return requestconfig.ExecuteNewRequest(ctx, http.MethodPatch, "v1/ios_instances/"+url.PathEscape(id), params, nil, opts...)
}

func (r *IosInstanceService) ListSessionArtifacts(ctx context.Context, id string, kind session.ArtifactKind, opts ...option.RequestOption) (res []session.Artifact, err error) {
	if kind != "" {
		opts = append(opts, option.WithQuery("kind", string(kind)))
	}
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, "v1/ios_instances/"+url.PathEscape(id)+"/session_artifacts", nil, &res, slices.Concat(r.Options, opts)...)
	return
}
func (r *IosInstanceService) ListRecordings(ctx context.Context, id string, opts ...option.RequestOption) ([]session.Artifact, error) {
	return r.ListSessionArtifacts(ctx, id, session.Recording, opts...)
}
func (r *IosInstanceService) ListAppLogs(ctx context.Context, id string, opts ...option.RequestOption) ([]session.Artifact, error) {
	return r.ListSessionArtifacts(ctx, id, session.AppLog, opts...)
}
func (r *IosInstanceService) ListEvents(ctx context.Context, id string, opts ...option.RequestOption) ([]session.Artifact, error) {
	return r.ListSessionArtifacts(ctx, id, session.EventLog, opts...)
}
