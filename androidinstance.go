// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package limrunv1

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/stainless-sdks/limrun-v1-go/internal/apijson"
	"github.com/stainless-sdks/limrun-v1-go/internal/apiquery"
	"github.com/stainless-sdks/limrun-v1-go/internal/requestconfig"
	"github.com/stainless-sdks/limrun-v1-go/option"
	"github.com/stainless-sdks/limrun-v1-go/packages/param"
	"github.com/stainless-sdks/limrun-v1-go/packages/respjson"
)

// AndroidInstanceService contains methods and other services that help with
// interacting with the limrun-v1 API.
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
	opts = append(r.Options[:], opts...)
	path := "v1/android_instances"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, params, &res, opts...)
	return
}

// Get Android instance with given ID
func (r *AndroidInstanceService) Get(ctx context.Context, id string, opts ...option.RequestOption) (res *AndroidInstance, err error) {
	opts = append(r.Options[:], opts...)
	if id == "" {
		err = errors.New("missing required id parameter")
		return
	}
	path := fmt.Sprintf("v1/android_instances/%s", id)
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)
	return
}

// List Android instances belonging to given organization
func (r *AndroidInstanceService) List(ctx context.Context, query AndroidInstanceListParams, opts ...option.RequestOption) (res *[]AndroidInstance, err error) {
	opts = append(r.Options[:], opts...)
	path := "v1/android_instances"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, query, &res, opts...)
	return
}

// Delete Android instance with given name
func (r *AndroidInstanceService) Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error) {
	opts = append(r.Options[:], opts...)
	opts = append([]option.RequestOption{option.WithHeader("Accept", "")}, opts...)
	if id == "" {
		err = errors.New("missing required id parameter")
		return
	}
	path := fmt.Sprintf("v1/android_instances/%s", id)
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)
	return
}

type AndroidInstance struct {
	Metadata AndroidInstanceMetadata `json:"metadata,required"`
	Spec     AndroidInstanceSpec     `json:"spec,required"`
	Status   AndroidInstanceStatus   `json:"status,required"`
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
func (r AndroidInstanceMetadata) RawJSON() string { return r.JSON.raw }
func (r *AndroidInstanceMetadata) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceSpec struct {
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
func (r AndroidInstanceSpec) RawJSON() string { return r.JSON.raw }
func (r *AndroidInstanceSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceStatus struct {
	Token                string `json:"token,required"`
	State                any    `json:"state,required"`
	AdbWebSocketURL      string `json:"adbWebSocketUrl"`
	EndpointWebSocketURL string `json:"endpointWebSocketUrl"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Token                respjson.Field
		State                respjson.Field
		AdbWebSocketURL      respjson.Field
		EndpointWebSocketURL respjson.Field
		ExtraFields          map[string]respjson.Field
		raw                  string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AndroidInstanceStatus) RawJSON() string { return r.JSON.raw }
func (r *AndroidInstanceStatus) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AndroidInstanceNewParams struct {
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
	// After how many minutes should the instance be terminated. Example values 1m,
	// 10m, 3h. Default is "0" which means no hard timeout.
	HardTimeout param.Opt[string] `json:"hardTimeout,omitzero" format:"duration"`
	// After how many minutes of inactivity should the instance be terminated. Example
	// values 1m, 10m, 3h. Default is 3m. Providing "0" disables inactivity checks
	// altogether.
	InactivityTimeout param.Opt[string] `json:"inactivityTimeout,omitzero" format:"duration"`
	// The region where the instance will be created. If not given, will be decided
	// based on scheduling clues and availability.
	Region        param.Opt[string]                          `json:"region,omitzero"`
	Clues         []AndroidInstanceNewParamsSpecClue         `json:"clues,omitzero"`
	InitialAssets []AndroidInstanceNewParamsSpecInitialAsset `json:"initialAssets,omitzero"`
	paramObj
}

func (r AndroidInstanceNewParamsSpec) MarshalJSON() (data []byte, err error) {
	type shadow AndroidInstanceNewParamsSpec
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AndroidInstanceNewParamsSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// The property Kind is required.
type AndroidInstanceNewParamsSpecClue struct {
	// Any of "ClientIP".
	Kind     string            `json:"kind,omitzero,required"`
	ClientIP param.Opt[string] `json:"clientIp,omitzero"`
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
		"kind", "ClientIP",
	)
}

// The properties Kind, Source are required.
type AndroidInstanceNewParamsSpecInitialAsset struct {
	// Any of "App".
	Kind string `json:"kind,omitzero,required"`
	// Any of "URL", "AssetName".
	Source    string            `json:"source,omitzero,required"`
	AssetName param.Opt[string] `json:"assetName,omitzero"`
	URL       param.Opt[string] `json:"url,omitzero"`
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
		"kind", "App",
	)
	apijson.RegisterFieldValidator[AndroidInstanceNewParamsSpecInitialAsset](
		"source", "URL", "AssetName",
	)
}

type AndroidInstanceListParams struct {
	// Labels filter to apply to Android instances to return. Expects a comma-separated
	// list of key=value pairs (e.g., env=prod,region=us-west).
	LabelSelector param.Opt[string] `query:"labelSelector,omitzero" json:"-"`
	// Region where the instance is scheduled on.
	Region param.Opt[string] `query:"region,omitzero" json:"-"`
	// State filter to apply to Android instances to return.
	State any `query:"state,omitzero" json:"-"`
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
