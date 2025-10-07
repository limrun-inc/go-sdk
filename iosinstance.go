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

	"github.com/limrun-inc/go-sdk/internal/apijson"
	"github.com/limrun-inc/go-sdk/internal/apiquery"
	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
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
	return
}

// List iOS instances
func (r *IosInstanceService) List(ctx context.Context, query IosInstanceListParams, opts ...option.RequestOption) (res *[]IosInstance, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/ios_instances"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, query, &res, opts...)
	return
}

// Delete iOS instance with given name
func (r *IosInstanceService) Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error) {
	opts = slices.Concat(r.Options, opts)
	opts = append([]option.RequestOption{option.WithHeader("Accept", "")}, opts...)
	if id == "" {
		err = errors.New("missing required id parameter")
		return
	}
	path := fmt.Sprintf("v1/ios_instances/%s", id)
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)
	return
}

// Get iOS instance with given ID
func (r *IosInstanceService) Get(ctx context.Context, id string, opts ...option.RequestOption) (res *IosInstance, err error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		err = errors.New("missing required id parameter")
		return
	}
	path := fmt.Sprintf("v1/ios_instances/%s", id)
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)
	return
}

type IosInstance struct {
	Metadata IosInstanceMetadata `json:"metadata,required"`
	Spec     IosInstanceSpec     `json:"spec,required"`
	Status   IosInstanceStatus   `json:"status,required"`
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
	ID             string            `json:"id,required"`
	CreatedAt      time.Time         `json:"createdAt,required" format:"date-time"`
	OrganizationID string            `json:"organizationId,required"`
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
	// After how many minutes of inactivity should the instance be terminated. Example
	// values 1m, 10m, 3h. Default is 3m. Providing "0" disables inactivity checks
	// altogether.
	InactivityTimeout string `json:"inactivityTimeout,required" format:"duration"`
	// The region where the instance will be created. If not given, will be decided
	// based on scheduling clues and availability.
	Region string `json:"region,required"`
	// After how many minutes should the instance be terminated. Example values 1m,
	// 10m, 3h. Default is "0" which means no hard timeout.
	HardTimeout string `json:"hardTimeout" format:"duration"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
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
	Token string `json:"token,required"`
	// Any of "unknown", "creating", "ready", "terminated".
	State                string `json:"state,required"`
	EndpointWebSocketURL string `json:"endpointWebSocketUrl"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Token                respjson.Field
		State                respjson.Field
		EndpointWebSocketURL respjson.Field
		ExtraFields          map[string]respjson.Field
		raw                  string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r IosInstanceStatus) RawJSON() string { return r.JSON.raw }
func (r *IosInstanceStatus) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type IosInstanceNewParams struct {
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
	// After how many minutes should the instance be terminated. Example values 1m,
	// 10m, 3h. Default is "0" which means no hard timeout.
	HardTimeout param.Opt[string] `json:"hardTimeout,omitzero" format:"duration"`
	// After how many minutes of inactivity should the instance be terminated. Example
	// values 1m, 10m, 3h. Default is 3m. Providing "0" disables inactivity checks
	// altogether.
	InactivityTimeout param.Opt[string] `json:"inactivityTimeout,omitzero" format:"duration"`
	// The region where the instance will be created. If not given, will be decided
	// based on scheduling clues and availability.
	Region        param.Opt[string]                      `json:"region,omitzero"`
	Clues         []IosInstanceNewParamsSpecClue         `json:"clues,omitzero"`
	InitialAssets []IosInstanceNewParamsSpecInitialAsset `json:"initialAssets,omitzero"`
	paramObj
}

func (r IosInstanceNewParamsSpec) MarshalJSON() (data []byte, err error) {
	type shadow IosInstanceNewParamsSpec
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *IosInstanceNewParamsSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// The property Kind is required.
type IosInstanceNewParamsSpecClue struct {
	// Any of "ClientIP".
	Kind     string            `json:"kind,omitzero,required"`
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
		"kind", "ClientIP",
	)
}

// The properties Kind, Source are required.
type IosInstanceNewParamsSpecInitialAsset struct {
	// Any of "App".
	Kind string `json:"kind,omitzero,required"`
	// Any of "URL", "AssetName".
	Source    string            `json:"source,omitzero,required"`
	AssetName param.Opt[string] `json:"assetName,omitzero"`
	URL       param.Opt[string] `json:"url,omitzero"`
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
		"kind", "App",
	)
	apijson.RegisterFieldValidator[IosInstanceNewParamsSpecInitialAsset](
		"source", "URL", "AssetName",
	)
}

type IosInstanceListParams struct {
	// Labels filter to apply to instances to return. Expects a comma-separated list of
	// key=value pairs (e.g., env=prod,region=us-west).
	LabelSelector param.Opt[string] `query:"labelSelector,omitzero" json:"-"`
	// Region where the instance is scheduled on.
	Region param.Opt[string] `query:"region,omitzero" json:"-"`
	// State filter to apply to instances to return.
	//
	// Any of "unknown", "creating", "ready", "terminated".
	State IosInstanceListParamsState `query:"state,omitzero" json:"-"`
	paramObj
}

// URLQuery serializes [IosInstanceListParams]'s query parameters as `url.Values`.
func (r IosInstanceListParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

// State filter to apply to instances to return.
type IosInstanceListParamsState string

const (
	IosInstanceListParamsStateUnknown    IosInstanceListParamsState = "unknown"
	IosInstanceListParamsStateCreating   IosInstanceListParamsState = "creating"
	IosInstanceListParamsStateReady      IosInstanceListParamsState = "ready"
	IosInstanceListParamsStateTerminated IosInstanceListParamsState = "terminated"
)
