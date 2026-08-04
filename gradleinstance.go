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
	"github.com/limrun-inc/go-sdk/packages/pagination"
	"github.com/limrun-inc/go-sdk/packages/param"
	"github.com/limrun-inc/go-sdk/packages/respjson"
)

// GradleInstanceService contains methods and other services that help with
// interacting with the limrun API.
//
// Note, unlike clients, this service does not read variables from the environment
// automatically. You should not instantiate this service directly, and instead use
// the [NewGradleInstanceService] method instead.
type GradleInstanceService struct {
	Options []option.RequestOption
}

// NewGradleInstanceService generates a new service that applies the given options
// to each request. These options are applied after the parent client's options (if
// there is one), and before any request-specific options.
func NewGradleInstanceService(opts ...option.RequestOption) (r GradleInstanceService) {
	r = GradleInstanceService{}
	r.Options = opts
	return
}

// Create a Gradle instance
func (r *GradleInstanceService) New(ctx context.Context, params GradleInstanceNewParams, opts ...option.RequestOption) (res *GradleInstance, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/gradle_instances"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, params, &res, opts...)
	return res, err
}

// List Gradle instances
func (r *GradleInstanceService) List(ctx context.Context, query GradleInstanceListParams, opts ...option.RequestOption) (res *pagination.Items[GradleInstance], err error) {
	var raw *http.Response
	opts = slices.Concat(r.Options, opts)
	opts = append([]option.RequestOption{option.WithResponseInto(&raw)}, opts...)
	path := "v1/gradle_instances"
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

// List Gradle instances
func (r *GradleInstanceService) ListAutoPaging(ctx context.Context, query GradleInstanceListParams, opts ...option.RequestOption) *pagination.ItemsAutoPager[GradleInstance] {
	return pagination.NewItemsAutoPager(r.List(ctx, query, opts...))
}

// Delete Gradle instance with given name
func (r *GradleInstanceService) Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error) {
	opts = slices.Concat(r.Options, opts)
	opts = append([]option.RequestOption{option.WithHeader("Accept", "*/*")}, opts...)
	if id == "" {
		err = errors.New("missing required id parameter")
		return err
	}
	path := fmt.Sprintf("v1/gradle_instances/%s", id)
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)
	return err
}

// Get Gradle instance with given ID
func (r *GradleInstanceService) Get(ctx context.Context, id string, opts ...option.RequestOption) (res *GradleInstance, err error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		err = errors.New("missing required id parameter")
		return nil, err
	}
	path := fmt.Sprintf("v1/gradle_instances/%s", id)
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)
	return res, err
}

type GradleInstance struct {
	Metadata GradleInstanceMetadata `json:"metadata" api:"required"`
	Spec     GradleInstanceSpec     `json:"spec" api:"required"`
	Status   GradleInstanceStatus   `json:"status" api:"required"`
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
func (r GradleInstance) RawJSON() string { return r.JSON.raw }
func (r *GradleInstance) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type GradleInstanceMetadata struct {
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
func (r GradleInstanceMetadata) RawJSON() string { return r.JSON.raw }
func (r *GradleInstanceMetadata) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type GradleInstanceSpec struct {
	InactivityTimeout string `json:"inactivityTimeout" api:"required" format:"duration"`
	Region            string `json:"region" api:"required"`
	HardTimeout       string `json:"hardTimeout" format:"duration"`
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
func (r GradleInstanceSpec) RawJSON() string { return r.JSON.raw }
func (r *GradleInstanceSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type GradleInstanceStatus struct {
	Token string `json:"token" api:"required"`
	// Any of "unknown", "creating", "assigned", "ready", "terminated".
	State        string `json:"state" api:"required"`
	APIURL       string `json:"apiUrl"`
	ErrorMessage string `json:"errorMessage"`
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
		Token             respjson.Field
		State             respjson.Field
		APIURL            respjson.Field
		ErrorMessage      respjson.Field
		TerminationReason respjson.Field
		ExtraFields       map[string]respjson.Field
		raw               string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r GradleInstanceStatus) RawJSON() string { return r.JSON.raw }
func (r *GradleInstanceStatus) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type GradleInstanceNewParams struct {
	// If there is another instance with given labels and region, return that one
	// instead of creating a new instance.
	ReuseIfExists param.Opt[bool] `query:"reuseIfExists,omitzero" json:"-"`
	// Return after the instance is ready to connect.
	Wait     param.Opt[bool]                 `query:"wait,omitzero" json:"-"`
	Metadata GradleInstanceNewParamsMetadata `json:"metadata,omitzero"`
	Spec     GradleInstanceNewParamsSpec     `json:"spec,omitzero"`
	paramObj
}

func (r GradleInstanceNewParams) MarshalJSON() (data []byte, err error) {
	type shadow GradleInstanceNewParams
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *GradleInstanceNewParams) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// URLQuery serializes [GradleInstanceNewParams]'s query parameters as
// `url.Values`.
func (r GradleInstanceNewParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

type GradleInstanceNewParamsMetadata struct {
	DisplayName param.Opt[string] `json:"displayName,omitzero"`
	Labels      map[string]string `json:"labels,omitzero"`
	paramObj
}

func (r GradleInstanceNewParamsMetadata) MarshalJSON() (data []byte, err error) {
	type shadow GradleInstanceNewParamsMetadata
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *GradleInstanceNewParamsMetadata) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type GradleInstanceNewParamsSpec struct {
	// After how many minutes should the instance be terminated. Example values 1m,
	// 10m, 3h. Default is "0" which means no hard timeout.
	HardTimeout param.Opt[string] `json:"hardTimeout,omitzero" format:"duration"`
	// After how many minutes of inactivity should the instance be terminated. The
	// timer starts once the instance becomes ready. Example values 1m, 10m, 3h.
	// Default is 5m; a non-positive value falls back to the default.
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
	Region param.Opt[string]                 `json:"region,omitzero"`
	Clues  []GradleInstanceNewParamsSpecClue `json:"clues,omitzero"`
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

func (r GradleInstanceNewParamsSpec) MarshalJSON() (data []byte, err error) {
	type shadow GradleInstanceNewParamsSpec
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *GradleInstanceNewParamsSpec) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[GradleInstanceNewParamsSpec](
		"jurisdiction", "us", "eu", "as",
	)
}

// The property Kind is required.
type GradleInstanceNewParamsSpecClue struct {
	// Any of "ClientIP".
	Kind     string            `json:"kind,omitzero" api:"required"`
	ClientIP param.Opt[string] `json:"clientIp,omitzero"`
	paramObj
}

func (r GradleInstanceNewParamsSpecClue) MarshalJSON() (data []byte, err error) {
	type shadow GradleInstanceNewParamsSpecClue
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *GradleInstanceNewParamsSpecClue) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func init() {
	apijson.RegisterFieldValidator[GradleInstanceNewParamsSpecClue](
		"kind", "ClientIP",
	)
}

type GradleInstanceListParams struct {
	EndingBefore param.Opt[string] `query:"endingBefore,omitzero" json:"-"`
	// Labels filter to apply to instances to return. Expects a comma-separated list of
	// key=value pairs (e.g., env=prod,region=us-west).
	LabelSelector param.Opt[string] `query:"labelSelector,omitzero" json:"-"`
	// Maximum number of items to be returned. The default is 50.
	Limit         param.Opt[int64]  `query:"limit,omitzero" json:"-"`
	StartingAfter param.Opt[string] `query:"startingAfter,omitzero" json:"-"`
	// State filter to apply to Gradle instances to return. Each comma-separated state
	// will be used as part of an OR clause, e.g. "assigned,ready" will return all
	// instances that are either assigned or ready.
	//
	// Valid states: creating, assigned, ready, terminated, unknown
	State param.Opt[string] `query:"state,omitzero" json:"-"`
	paramObj
}

// URLQuery serializes [GradleInstanceListParams]'s query parameters as
// `url.Values`.
func (r GradleInstanceListParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}
