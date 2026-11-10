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

// AndroidInstanceService contains methods and other services that help with
// interacting with the limrun API.
//
// Note, unlike clients, this service does not read variables from the environment
// automatically. You should not instantiate this service directly, and instead use
// the [NewAndroidInstanceService] method instead.
type AndroidInstanceService struct {
	Options []option.RequestOption
}

// NewAndroidInstanceService generates a new service that applies the given options
// to each request. These options are applied after the parent client's options (if
// there is one), and before any request-specific options.
func NewAndroidInstanceService(opts ...option.RequestOption) (r AndroidInstanceService) {
	r = AndroidInstanceService{}
	r.Options = opts
	return
}

// Create an Android instance
func (r *AndroidInstanceService) New(ctx context.Context, params AndroidInstanceNewParams, opts ...option.RequestOption) (res *AndroidInstance, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/android_instances"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, params, &res, opts...)
	return res, err
}

// List Android instances
func (r *AndroidInstanceService) List(ctx context.Context, query AndroidInstanceListParams, opts ...option.RequestOption) (res *pagination.Items[AndroidInstance], err error) {
	var raw *http.Response
	opts = slices.Concat(r.Options, opts)
	opts = append([]option.RequestOption{option.WithResponseInto(&raw)}, opts...)
	path := "v1/android_instances"
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

// List Android instances
func (r *AndroidInstanceService) ListAutoPaging(ctx context.Context, query AndroidInstanceListParams, opts ...option.RequestOption) *pagination.ItemsAutoPager[AndroidInstance] {
	return pagination.NewItemsAutoPager(r.List(ctx, query, opts...))
}

// Delete Android instance with given name
func (r *AndroidInstanceService) Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error) {
	opts = slices.Concat(r.Options, opts)
	opts = append([]option.RequestOption{option.WithHeader("Accept", "*/*")}, opts...)
	if id == "" {
		err = errors.New("missing required id parameter")
		return err
	}
	path := fmt.Sprintf("v1/android_instances/%s", url.PathEscape(id))
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)
	return err
}

// Get Android instance with given ID
func (r *AndroidInstanceService) Get(ctx context.Context, id string, opts ...option.RequestOption) (res *AndroidInstance, err error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		err = errors.New("missing required id parameter")
		return nil, err
	}
	path := fmt.Sprintf("v1/android_instances/%s", url.PathEscape(id))
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)
	return res, err
}

type AndroidInstance struct {
	Metadata AndroidInstanceMetadata `json:"metadata" api:"required"`
	Spec     AndroidInstanceSpec     `json:"spec" api:"required"`
	Status   AndroidInstanceStatus   `json:"status" api:"required"`
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
func (r AndroidInstance) RawJSON() string { return r.JSON.raw }
func (r *AndroidInstance) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceMetadata struct {
	ID             string            `json:"id" api:"required"`
	CreatedAt      time.Time         `json:"createdAt" api:"required" format:"date-time"`
	OrganizationID string            `json:"organizationId" api:"required"`
	DisplayName    string            `json:"displayName"`
	Labels         map[string]string `json:"labels"`
	TerminatedAt   time.Time         `json:"terminatedAt" format:"date-time"`
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
		TerminatedAt   respjson.Field
		TerminatingAt  respjson.Field
		ExtraFields    map[string]respjson.Field
		raw            string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AndroidInstanceMetadata) RawJSON() string { return r.JSON.raw }
func (r *AndroidInstanceMetadata) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceSpec struct {
	// Current effective permissions. Present on live GETs; historical records may omit it.
	Role AndroidInstanceSpecRole `json:"role"`
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
func (r AndroidInstanceSpec) RawJSON() string { return r.JSON.raw }
func (r *AndroidInstanceSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceStatus struct {
	// A signed token that grants only streaming on this instance: it opens the
	// screen-streaming WebSocket and nothing else, so a browser can hold it while
	// `token` stays on your backend. Create, get and list return it to callers with
	// full control of the instance; get and list only while the instance runs.
	StreamToken string `json:"streamToken"`

	Token string `json:"token"`
	// Any of "unknown", "creating", "assigned", "ready", "terminating", "terminated".
	State                   string                       `json:"state" api:"required"`
	AdbWebSocketURL         string                       `json:"adbWebSocketUrl"`
	APIURL                  string                       `json:"apiUrl"`
	EndpointWebSocketURL    string                       `json:"endpointWebSocketUrl"`
	ErrorMessage            string                       `json:"errorMessage"`
	McpURL                  string                       `json:"mcpUrl"`
	Sandbox                 AndroidInstanceStatusSandbox `json:"sandbox"`
	SignedStreamURL         string                       `json:"signedStreamUrl"`
	TargetHTTPPortURLPrefix string                       `json:"targetHttpPortUrlPrefix"`
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
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		StreamToken respjson.Field

		Token                   respjson.Field
		State                   respjson.Field
		AdbWebSocketURL         respjson.Field
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
func (r AndroidInstanceStatus) RawJSON() string { return r.JSON.raw }
func (r *AndroidInstanceStatus) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceStatusSandbox struct {
	PlaywrightAndroid AndroidInstanceStatusSandboxPlaywrightAndroid `json:"playwrightAndroid"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		PlaywrightAndroid respjson.Field
		ExtraFields       map[string]respjson.Field
		raw               string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AndroidInstanceStatusSandbox) RawJSON() string { return r.JSON.raw }
func (r *AndroidInstanceStatusSandbox) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceStatusSandboxPlaywrightAndroid struct {
	URL string `json:"url"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		URL         respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AndroidInstanceStatusSandboxPlaywrightAndroid) RawJSON() string { return r.JSON.raw }
func (r *AndroidInstanceStatusSandboxPlaywrightAndroid) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceNewParams struct {

	// If there is another instance with given labels and region, return that one
	// instead of creating a new instance.
	ReuseIfExists param.Opt[bool] `query:"reuseIfExists,omitzero" json:"-"`
	// Return after the instance is ready to connect.
	Wait     param.Opt[bool]                  `query:"wait,omitzero" json:"-"`
	Metadata AndroidInstanceNewParamsMetadata `json:"metadata,omitzero"`
	Spec     AndroidInstanceNewParamsSpec     `json:"spec,omitzero"`
	paramObj
}

func (r AndroidInstanceNewParams) MarshalJSON() (data []byte, err error) {
	type shadow AndroidInstanceNewParams
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AndroidInstanceNewParams) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// URLQuery serializes [AndroidInstanceNewParams]'s query parameters as
// `url.Values`.
func (r AndroidInstanceNewParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

type AndroidInstanceNewParamsMetadata struct {
	DisplayName param.Opt[string] `json:"displayName,omitzero"`
	Labels      map[string]string `json:"labels,omitzero"`
	paramObj
}

func (r AndroidInstanceNewParamsMetadata) MarshalJSON() (data []byte, err error) {
	type shadow AndroidInstanceNewParamsMetadata
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AndroidInstanceNewParamsMetadata) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceNewParamsSpec struct {
	// Inline instance role. Omit for the organization's compatibility default.
	Role AndroidInstanceNewParamsSpecRole `json:"role,omitzero"`
	// The Android device model. Default is phone.
	Model param.Opt[string] `json:"model,omitzero"`
	// Names a private tunnel, created in the console (Network) or the API, that
	// serves this instance. Creation fails while no connector (`lim tunnel run`)
	// holds it, and the instance becomes ready only after the tunnel attaches.
	Tunnel param.Opt[string] `json:"tunnel,omitzero"`

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
	Region        param.Opt[string]                          `json:"region,omitzero"`
	Clues         []AndroidInstanceNewParamsSpecClue         `json:"clues,omitzero"`
	InitialAssets []AndroidInstanceNewParamsSpecInitialAsset `json:"initialAssets,omitzero"`
	// Restricts scheduling to regions in the given jurisdiction. Unlike region, this
	// is a hard constraint: the request never overflows to a region outside the
	// jurisdiction and fails when no region in the jurisdiction has capacity. A region
	// belongs to a jurisdiction when its name starts with the jurisdiction prefix,
	// e.g. "eu-north1" is in "eu". A region preference pointing outside the
	// jurisdiction is ignored.
	//
	// Any of "us", "eu", "as".
	Jurisdiction string                              `json:"jurisdiction,omitzero"`
	Sandbox      AndroidInstanceNewParamsSpecSandbox `json:"sandbox,omitzero"`
	paramObj
}

func (r AndroidInstanceNewParamsSpec) MarshalJSON() (data []byte, err error) {
	type shadow AndroidInstanceNewParamsSpec
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AndroidInstanceNewParamsSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[AndroidInstanceNewParamsSpec](
		"jurisdiction", "us", "eu", "as",
	)
}

// The property Kind is required.
type AndroidInstanceNewParamsSpecClue struct {
	// Required for ClientLocation. Both coordinates must be finite and within their
	// ranges. This location takes precedence over ClientIP.
	ClientLocation *AndroidInstanceNewParamsSpecClueClientLocation `json:"clientLocation,omitzero"`

	// Any of "ClientIP", "OSVersion".
	Kind     string            `json:"kind,omitzero" api:"required"`
	ClientIP param.Opt[string] `json:"clientIp,omitzero"`
	// The major version of Android, e.g. "13", "14" or "15".
	OsVersion param.Opt[string] `json:"osVersion,omitzero"`
	paramObj
}

func (r AndroidInstanceNewParamsSpecClue) MarshalJSON() (data []byte, err error) {
	type shadow AndroidInstanceNewParamsSpecClue
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AndroidInstanceNewParamsSpecClue) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[AndroidInstanceNewParamsSpecClue](
		"kind", "ClientIP", "ClientLocation", "OSVersion", "OSVersion",
	)
}

// The property Kind is required.
type AndroidInstanceNewParamsSpecInitialAsset struct {

	// Any of "App", "Configuration".
	Kind          string                                                `json:"kind,omitzero" api:"required"`
	AssetName     param.Opt[string]                                     `json:"assetName,omitzero"`
	URL           param.Opt[string]                                     `json:"url,omitzero"`
	AssetIDs      []string                                              `json:"assetIds,omitzero"`
	AssetNames    []string                                              `json:"assetNames,omitzero"`
	Configuration AndroidInstanceNewParamsSpecInitialAssetConfiguration `json:"configuration,omitzero"`
	// Any of "URL", "URLs", "AssetName", "AssetNames", "AssetIDs".
	Source string   `json:"source,omitzero"`
	URLs   []string `json:"urls,omitzero"`
	paramObj
}

func (r AndroidInstanceNewParamsSpecInitialAsset) MarshalJSON() (data []byte, err error) {
	type shadow AndroidInstanceNewParamsSpecInitialAsset
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AndroidInstanceNewParamsSpecInitialAsset) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[AndroidInstanceNewParamsSpecInitialAsset](
		"kind", "App", "Configuration",
	)
	apijson.RegisterFieldValidator[AndroidInstanceNewParamsSpecInitialAsset](
		"source", "URL", "URLs", "AssetName", "AssetNames", "AssetIDs",
	)
}

// The property Kind is required.
type AndroidInstanceNewParamsSpecInitialAssetConfiguration struct {

	// Any of "ChromeFlag".
	Kind string `json:"kind,omitzero" api:"required"`
	// Any of "enable-command-line-on-non-rooted-devices@1".
	ChromeFlag string `json:"chromeFlag,omitzero"`
	paramObj
}

func (r AndroidInstanceNewParamsSpecInitialAssetConfiguration) MarshalJSON() (data []byte, err error) {
	type shadow AndroidInstanceNewParamsSpecInitialAssetConfiguration
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AndroidInstanceNewParamsSpecInitialAssetConfiguration) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[AndroidInstanceNewParamsSpecInitialAssetConfiguration](
		"kind", "ChromeFlag",
	)
	apijson.RegisterFieldValidator[AndroidInstanceNewParamsSpecInitialAssetConfiguration](
		"chromeFlag", "enable-command-line-on-non-rooted-devices@1",
	)
}

type AndroidInstanceNewParamsSpecSandbox struct {
	PlaywrightAndroid AndroidInstanceNewParamsSpecSandboxPlaywrightAndroid `json:"playwrightAndroid,omitzero"`
	paramObj
}

func (r AndroidInstanceNewParamsSpecSandbox) MarshalJSON() (data []byte, err error) {
	type shadow AndroidInstanceNewParamsSpecSandbox
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AndroidInstanceNewParamsSpecSandbox) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceNewParamsSpecSandboxPlaywrightAndroid struct {
	Enabled param.Opt[bool] `json:"enabled,omitzero"`
	// Any of "1.56.1-lim.1", "1.60.0-lim.1".
	Version string `json:"version,omitzero"`
	paramObj
}

func (r AndroidInstanceNewParamsSpecSandboxPlaywrightAndroid) MarshalJSON() (data []byte, err error) {
	type shadow AndroidInstanceNewParamsSpecSandboxPlaywrightAndroid
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AndroidInstanceNewParamsSpecSandboxPlaywrightAndroid) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[AndroidInstanceNewParamsSpecSandboxPlaywrightAndroid](
		"version", "1.56.1-lim.1", "1.60.0-lim.1",
	)
}

type AndroidInstanceListParams struct {
	EndingBefore param.Opt[string] `query:"endingBefore,omitzero" json:"-"`
	// Labels filter to apply to Android instances to return. Expects a comma-separated
	// list of key=value pairs (e.g., env=prod,region=us-west).
	LabelSelector param.Opt[string] `query:"labelSelector,omitzero" json:"-"`
	// Maximum number of instances to be returned. The default is 50.
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

// URLQuery serializes [AndroidInstanceListParams]'s query parameters as
// `url.Values`.
func (r AndroidInstanceListParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

type AndroidInstanceSpecRole struct {
	Permissions []string `json:"permissions"`
	JSON        struct {
		Permissions respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

func (r AndroidInstanceSpecRole) RawJSON() string { return r.JSON.raw }
func (r *AndroidInstanceSpecRole) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceUpdateParams struct {
	// Required quoted ETag from a live GET. Missing/empty values are rejected;
	// a concurrent change returns HTTP 412. Unconditional updates are not supported.
	IfMatch string `json:"-"`
	// Replaces all customer labels. Use {} to clear them. Omission preserves labels.
	Metadata AndroidInstanceUpdateParamsMetadata `json:"metadata,omitzero"`
	Spec     AndroidInstanceUpdateParamsSpec     `json:"spec,omitzero"`
	paramObj
}

func (r AndroidInstanceUpdateParams) MarshalJSON() ([]byte, error) {
	type shadow AndroidInstanceUpdateParams
	return param.MarshalObject(r, (*shadow)(&r))
}

type AndroidInstanceUpdateParamsMetadata struct {
	Labels map[string]string `json:"labels,omitzero"`
	paramObj
}

func (r AndroidInstanceUpdateParamsMetadata) MarshalJSON() ([]byte, error) {
	type shadow AndroidInstanceUpdateParamsMetadata
	return param.MarshalObject(r, (*shadow)(&r))
}

type AndroidInstanceUpdateParamsSpec struct {
	Role AndroidInstanceUpdateParamsSpecRole `json:"role,omitzero"`
	paramObj
}

func (r AndroidInstanceUpdateParamsSpec) MarshalJSON() ([]byte, error) {
	type shadow AndroidInstanceUpdateParamsSpec
	return param.MarshalObject(r, (*shadow)(&r))
}

type AndroidInstanceUpdateParamsSpecRole struct {
	// Replaces the complete list. Use [] to remove delegated permissions.
	Permissions []string `json:"permissions,omitzero"`
	paramObj
}

func (r AndroidInstanceUpdateParamsSpecRole) MarshalJSON() ([]byte, error) {
	type shadow AndroidInstanceUpdateParamsSpecRole
	return param.MarshalObject(r, (*shadow)(&r))
}

type AndroidInstanceNewParamsSpecRole struct {
	// Asset permissions delegated by the creator. An empty array grants nothing.
	Permissions []string `json:"permissions,omitzero"`
	paramObj
}

func (r AndroidInstanceNewParamsSpecRole) MarshalJSON() ([]byte, error) {
	type shadow AndroidInstanceNewParamsSpecRole
	return param.MarshalObject(r, (*shadow)(&r))
}

type AndroidInstanceNewParamsSpecClueClientLocation struct {
	// Latitude in decimal degrees, from -90 to 90 inclusive. Zero is valid.
	Latitude float64 `json:"latitude" api:"required"`
	// Longitude in decimal degrees, from -180 to 180 inclusive. Zero is valid.
	Longitude float64 `json:"longitude" api:"required"`
	paramObj
}

func (r AndroidInstanceNewParamsSpecClueClientLocation) MarshalJSON() ([]byte, error) {
	type shadow AndroidInstanceNewParamsSpecClueClientLocation
	return param.MarshalObject(r, (*shadow)(&r))
}

func (r *AndroidInstanceService) Update(ctx context.Context, id string, params AndroidInstanceUpdateParams, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts, []option.RequestOption{option.WithHeader("If-Match", params.IfMatch), option.WithHeader("Accept", "*/*")})
	return requestconfig.ExecuteNewRequest(ctx, http.MethodPatch, "v1/android_instances/"+url.PathEscape(id), params, nil, opts...)
}

func (r *AndroidInstanceService) ListSessionArtifacts(ctx context.Context, id string, kind session.ArtifactKind, opts ...option.RequestOption) (res []session.Artifact, err error) {
	if kind != "" {
		opts = append(opts, option.WithQuery("kind", string(kind)))
	}
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, "v1/android_instances/"+url.PathEscape(id)+"/session_artifacts", nil, &res, slices.Concat(r.Options, opts)...)
	return
}
func (r *AndroidInstanceService) ListRecordings(ctx context.Context, id string, opts ...option.RequestOption) ([]session.Artifact, error) {
	return r.ListSessionArtifacts(ctx, id, session.Recording, opts...)
}
func (r *AndroidInstanceService) ListAppLogs(ctx context.Context, id string, opts ...option.RequestOption) ([]session.Artifact, error) {
	return r.ListSessionArtifacts(ctx, id, session.AppLog, opts...)
}
func (r *AndroidInstanceService) ListEvents(ctx context.Context, id string, opts ...option.RequestOption) ([]session.Artifact, error) {
	return r.ListSessionArtifacts(ctx, id, session.EventLog, opts...)
}
