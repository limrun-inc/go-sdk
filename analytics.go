// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package limrun

import (
	"context"
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

// AnalyticsService contains methods and other services that help with interacting
// with the limrun API.
//
// Note, unlike clients, this service does not read variables from the environment
// automatically. You should not instantiate this service directly, and instead use
// the [NewAnalyticsService] method instead.
type AnalyticsService struct {
	Options []option.RequestOption
}

// NewAnalyticsService generates a new service that applies the given options to
// each request. These options are applied after the parent client's options (if
// there is one), and before any request-specific options.
func NewAnalyticsService(opts ...option.RequestOption) (r AnalyticsService) {
	r = AnalyticsService{}
	r.Options = opts
	return
}

// Get analytics for the authenticated organization
func (r *AnalyticsService) Get(ctx context.Context, query AnalyticsGetParams, opts ...option.RequestOption) (res *AnalyticsResponse, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/analytics"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, query, &res, opts...)
	return res, err
}

// Returns per-instance analytics grouped by minute bucket for detailed chart
// views.
func (r *AnalyticsService) GetInstances(ctx context.Context, query AnalyticsGetInstancesParams, opts ...option.RequestOption) (res *AnalyticsInstancesResponse, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/analytics/instances"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, query, &res, opts...)
	return res, err
}

type AnalyticsInstancesResponse struct {
	AsOf   time.Time                          `json:"asOf" api:"required" format:"date-time"`
	From   time.Time                          `json:"from" api:"required" format:"date-time"`
	Series []AnalyticsInstancesResponseSeries `json:"series" api:"required"`
	// IANA timezone used for time bucket grouping
	Timezone string    `json:"timezone" api:"required"`
	To       time.Time `json:"to" api:"required" format:"date-time"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		AsOf        respjson.Field
		From        respjson.Field
		Series      respjson.Field
		Timezone    respjson.Field
		To          respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsInstancesResponse) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsInstancesResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AnalyticsInstancesResponseSeries struct {
	Instances []AnalyticsInstancesResponseSeriesInstance `json:"instances" api:"required"`
	// RFC3339 timestamp for the start of the minute bucket in the requested timezone,
	// including the local offset
	Timestamp string `json:"timestamp" api:"required"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Instances   respjson.Field
		Timestamp   respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsInstancesResponseSeries) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsInstancesResponseSeries) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Analytics details for a single instance within a time bucket
type AnalyticsInstancesResponseSeriesInstance struct {
	// Billed minutes with platform multiplier applied
	BilledMinutes int64 `json:"billedMinutes" api:"required"`
	// Total cost in dollars for this instance
	Cost float64 `json:"cost" api:"required"`
	// Instance type ID (e.g., ios_xxx, android_xxx)
	InstanceTid string `json:"instanceTid" api:"required"`
	// Platform name, such as android, ios, or xcode
	Platform string `json:"platform" api:"required"`
	// Actual runtime minutes before platform multiplier
	RuntimeMinutes  int64                                                   `json:"runtimeMinutes" api:"required"`
	BilledBreakdown AnalyticsInstancesResponseSeriesInstanceBilledBreakdown `json:"billedBreakdown"`
	// Cost breakdown by billing source in dollars
	CostBreakdown AnalyticsInstancesResponseSeriesInstanceCostBreakdown `json:"costBreakdown"`
	// Instance labels at billing time
	Labels map[string]string `json:"labels"`
	// Region where the instance ran
	Region string `json:"region"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		BilledMinutes   respjson.Field
		Cost            respjson.Field
		InstanceTid     respjson.Field
		Platform        respjson.Field
		RuntimeMinutes  respjson.Field
		BilledBreakdown respjson.Field
		CostBreakdown   respjson.Field
		Labels          respjson.Field
		Region          respjson.Field
		ExtraFields     map[string]respjson.Field
		raw             string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsInstancesResponseSeriesInstance) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsInstancesResponseSeriesInstance) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AnalyticsInstancesResponseSeriesInstanceBilledBreakdown struct {
	CreditsBilledMinutes  int64 `json:"creditsBilledMinutes" api:"required"`
	OnDemandBilledMinutes int64 `json:"onDemandBilledMinutes" api:"required"`
	// Map of plan ID to billed minutes
	PlanBilledMinutes map[string]int64 `json:"planBilledMinutes"`
	// Map of subscription ID to billed minutes
	SubscriptionBilledMinutes map[string]int64 `json:"subscriptionBilledMinutes"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		CreditsBilledMinutes      respjson.Field
		OnDemandBilledMinutes     respjson.Field
		PlanBilledMinutes         respjson.Field
		SubscriptionBilledMinutes respjson.Field
		ExtraFields               map[string]respjson.Field
		raw                       string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsInstancesResponseSeriesInstanceBilledBreakdown) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsInstancesResponseSeriesInstanceBilledBreakdown) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Cost breakdown by billing source in dollars
type AnalyticsInstancesResponseSeriesInstanceCostBreakdown struct {
	// Cost from credits (always 0)
	CreditsCost float64 `json:"creditsCost" api:"required"`
	// Cost from on-demand billing in dollars
	OnDemandCost float64 `json:"onDemandCost" api:"required"`
	// Map of plan ID to cost in dollars
	PlanCost map[string]float64 `json:"planCost"`
	// Map of subscription ID to cost in dollars
	SubscriptionCost map[string]float64 `json:"subscriptionCost"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		CreditsCost      respjson.Field
		OnDemandCost     respjson.Field
		PlanCost         respjson.Field
		SubscriptionCost respjson.Field
		ExtraFields      map[string]respjson.Field
		raw              string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsInstancesResponseSeriesInstanceCostBreakdown) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsInstancesResponseSeriesInstanceCostBreakdown) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AnalyticsResponse struct {
	AsOf time.Time `json:"asOf" api:"required" format:"date-time"`
	// Any of "hour", "day", "week", "minute".
	Bucket AnalyticsResponseBucket   `json:"bucket" api:"required"`
	From   time.Time                 `json:"from" api:"required" format:"date-time"`
	Series []AnalyticsResponseSeries `json:"series" api:"required"`
	// Summary of analytics across all time buckets, broken down by platform and region
	Summary AnalyticsResponseSummary `json:"summary" api:"required"`
	// IANA timezone used for time bucket grouping
	Timezone string    `json:"timezone" api:"required"`
	To       time.Time `json:"to" api:"required" format:"date-time"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		AsOf        respjson.Field
		Bucket      respjson.Field
		From        respjson.Field
		Series      respjson.Field
		Summary     respjson.Field
		Timezone    respjson.Field
		To          respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponse) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AnalyticsResponseBucket string

const (
	AnalyticsResponseBucketHour   AnalyticsResponseBucket = "hour"
	AnalyticsResponseBucketDay    AnalyticsResponseBucket = "day"
	AnalyticsResponseBucketWeek   AnalyticsResponseBucket = "week"
	AnalyticsResponseBucketMinute AnalyticsResponseBucket = "minute"
)

// Analytics data for a single time bucket, broken down by platform and region
type AnalyticsResponseSeries struct {
	// Map of region to analytics stats for Android
	Android map[string]AnalyticsResponseSeriesAndroid `json:"android" api:"required"`
	// Map of region to analytics stats for iOS
	Ios map[string]AnalyticsResponseSeriesIo `json:"ios" api:"required"`
	// RFC3339 timestamp for the start of the bucket in the requested timezone,
	// including the local offset
	Timestamp string `json:"timestamp" api:"required"`
	// Map of region to analytics stats for Xcode
	Xcode map[string]AnalyticsResponseSeriesXcode `json:"xcode" api:"required"`
	// Individual instance details for this time bucket
	Instances []AnalyticsResponseSeriesInstance `json:"instances"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Android     respjson.Field
		Ios         respjson.Field
		Timestamp   respjson.Field
		Xcode       respjson.Field
		Instances   respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSeries) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSeries) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Complete analytics for a specific region including billing breakdown
type AnalyticsResponseSeriesAndroid struct {
	// Average instance duration in minutes
	AvgDurationMinutes float64 `json:"avgDurationMinutes" api:"required"`
	// Billed minutes with platform multiplier applied
	BilledMinutes int64 `json:"billedMinutes" api:"required"`
	// Total cost in dollars
	Cost float64 `json:"cost" api:"required"`
	// Number of unique instances
	Count int64 `json:"count" api:"required"`
	// Minutes billed to credits
	CreditsBilledMinutes int64 `json:"creditsBilledMinutes" api:"required"`
	// Cost from credits (always 0)
	CreditsCost float64 `json:"creditsCost" api:"required"`
	// Minutes billed on-demand
	OnDemandBilledMinutes int64 `json:"onDemandBilledMinutes" api:"required"`
	// Cost from on-demand billing in dollars
	OnDemandCost float64 `json:"onDemandCost" api:"required"`
	// Actual runtime minutes before platform multiplier
	RuntimeMinutes int64 `json:"runtimeMinutes" api:"required"`
	// Map of subscription ID to billed minutes
	SubscriptionBilledMinutes map[string]int64 `json:"subscriptionBilledMinutes"`
	// Map of subscription ID to cost in dollars
	SubscriptionCost map[string]float64 `json:"subscriptionCost"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		AvgDurationMinutes        respjson.Field
		BilledMinutes             respjson.Field
		Cost                      respjson.Field
		Count                     respjson.Field
		CreditsBilledMinutes      respjson.Field
		CreditsCost               respjson.Field
		OnDemandBilledMinutes     respjson.Field
		OnDemandCost              respjson.Field
		RuntimeMinutes            respjson.Field
		SubscriptionBilledMinutes respjson.Field
		SubscriptionCost          respjson.Field
		ExtraFields               map[string]respjson.Field
		raw                       string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSeriesAndroid) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSeriesAndroid) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Complete analytics for a specific region including billing breakdown
type AnalyticsResponseSeriesIo struct {
	// Average instance duration in minutes
	AvgDurationMinutes float64 `json:"avgDurationMinutes" api:"required"`
	// Billed minutes with platform multiplier applied
	BilledMinutes int64 `json:"billedMinutes" api:"required"`
	// Total cost in dollars
	Cost float64 `json:"cost" api:"required"`
	// Number of unique instances
	Count int64 `json:"count" api:"required"`
	// Minutes billed to credits
	CreditsBilledMinutes int64 `json:"creditsBilledMinutes" api:"required"`
	// Cost from credits (always 0)
	CreditsCost float64 `json:"creditsCost" api:"required"`
	// Minutes billed on-demand
	OnDemandBilledMinutes int64 `json:"onDemandBilledMinutes" api:"required"`
	// Cost from on-demand billing in dollars
	OnDemandCost float64 `json:"onDemandCost" api:"required"`
	// Actual runtime minutes before platform multiplier
	RuntimeMinutes int64 `json:"runtimeMinutes" api:"required"`
	// Map of subscription ID to billed minutes
	SubscriptionBilledMinutes map[string]int64 `json:"subscriptionBilledMinutes"`
	// Map of subscription ID to cost in dollars
	SubscriptionCost map[string]float64 `json:"subscriptionCost"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		AvgDurationMinutes        respjson.Field
		BilledMinutes             respjson.Field
		Cost                      respjson.Field
		Count                     respjson.Field
		CreditsBilledMinutes      respjson.Field
		CreditsCost               respjson.Field
		OnDemandBilledMinutes     respjson.Field
		OnDemandCost              respjson.Field
		RuntimeMinutes            respjson.Field
		SubscriptionBilledMinutes respjson.Field
		SubscriptionCost          respjson.Field
		ExtraFields               map[string]respjson.Field
		raw                       string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSeriesIo) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSeriesIo) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Complete analytics for a specific region including billing breakdown
type AnalyticsResponseSeriesXcode struct {
	// Average instance duration in minutes
	AvgDurationMinutes float64 `json:"avgDurationMinutes" api:"required"`
	// Billed minutes with platform multiplier applied
	BilledMinutes int64 `json:"billedMinutes" api:"required"`
	// Total cost in dollars
	Cost float64 `json:"cost" api:"required"`
	// Number of unique instances
	Count int64 `json:"count" api:"required"`
	// Minutes billed to credits
	CreditsBilledMinutes int64 `json:"creditsBilledMinutes" api:"required"`
	// Cost from credits (always 0)
	CreditsCost float64 `json:"creditsCost" api:"required"`
	// Minutes billed on-demand
	OnDemandBilledMinutes int64 `json:"onDemandBilledMinutes" api:"required"`
	// Cost from on-demand billing in dollars
	OnDemandCost float64 `json:"onDemandCost" api:"required"`
	// Actual runtime minutes before platform multiplier
	RuntimeMinutes int64 `json:"runtimeMinutes" api:"required"`
	// Map of subscription ID to billed minutes
	SubscriptionBilledMinutes map[string]int64 `json:"subscriptionBilledMinutes"`
	// Map of subscription ID to cost in dollars
	SubscriptionCost map[string]float64 `json:"subscriptionCost"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		AvgDurationMinutes        respjson.Field
		BilledMinutes             respjson.Field
		Cost                      respjson.Field
		Count                     respjson.Field
		CreditsBilledMinutes      respjson.Field
		CreditsCost               respjson.Field
		OnDemandBilledMinutes     respjson.Field
		OnDemandCost              respjson.Field
		RuntimeMinutes            respjson.Field
		SubscriptionBilledMinutes respjson.Field
		SubscriptionCost          respjson.Field
		ExtraFields               map[string]respjson.Field
		raw                       string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSeriesXcode) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSeriesXcode) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Analytics details for a single instance within a time bucket
type AnalyticsResponseSeriesInstance struct {
	// Billed minutes with platform multiplier applied
	BilledMinutes int64 `json:"billedMinutes" api:"required"`
	// Total cost in dollars for this instance
	Cost float64 `json:"cost" api:"required"`
	// Instance type ID (e.g., ios_xxx, android_xxx)
	InstanceTid string `json:"instanceTid" api:"required"`
	// Platform name, such as android, ios, or xcode
	Platform string `json:"platform" api:"required"`
	// Actual runtime minutes before platform multiplier
	RuntimeMinutes  int64                                          `json:"runtimeMinutes" api:"required"`
	BilledBreakdown AnalyticsResponseSeriesInstanceBilledBreakdown `json:"billedBreakdown"`
	// Cost breakdown by billing source in dollars
	CostBreakdown AnalyticsResponseSeriesInstanceCostBreakdown `json:"costBreakdown"`
	// Instance labels at billing time
	Labels map[string]string `json:"labels"`
	// Region where the instance ran
	Region string `json:"region"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		BilledMinutes   respjson.Field
		Cost            respjson.Field
		InstanceTid     respjson.Field
		Platform        respjson.Field
		RuntimeMinutes  respjson.Field
		BilledBreakdown respjson.Field
		CostBreakdown   respjson.Field
		Labels          respjson.Field
		Region          respjson.Field
		ExtraFields     map[string]respjson.Field
		raw             string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSeriesInstance) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSeriesInstance) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AnalyticsResponseSeriesInstanceBilledBreakdown struct {
	CreditsBilledMinutes  int64 `json:"creditsBilledMinutes" api:"required"`
	OnDemandBilledMinutes int64 `json:"onDemandBilledMinutes" api:"required"`
	// Map of plan ID to billed minutes
	PlanBilledMinutes map[string]int64 `json:"planBilledMinutes"`
	// Map of subscription ID to billed minutes
	SubscriptionBilledMinutes map[string]int64 `json:"subscriptionBilledMinutes"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		CreditsBilledMinutes      respjson.Field
		OnDemandBilledMinutes     respjson.Field
		PlanBilledMinutes         respjson.Field
		SubscriptionBilledMinutes respjson.Field
		ExtraFields               map[string]respjson.Field
		raw                       string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSeriesInstanceBilledBreakdown) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSeriesInstanceBilledBreakdown) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Cost breakdown by billing source in dollars
type AnalyticsResponseSeriesInstanceCostBreakdown struct {
	// Cost from credits (always 0)
	CreditsCost float64 `json:"creditsCost" api:"required"`
	// Cost from on-demand billing in dollars
	OnDemandCost float64 `json:"onDemandCost" api:"required"`
	// Map of plan ID to cost in dollars
	PlanCost map[string]float64 `json:"planCost"`
	// Map of subscription ID to cost in dollars
	SubscriptionCost map[string]float64 `json:"subscriptionCost"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		CreditsCost      respjson.Field
		OnDemandCost     respjson.Field
		PlanCost         respjson.Field
		SubscriptionCost respjson.Field
		ExtraFields      map[string]respjson.Field
		raw              string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSeriesInstanceCostBreakdown) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSeriesInstanceCostBreakdown) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Summary of analytics across all time buckets, broken down by platform and region
type AnalyticsResponseSummary struct {
	// Map of region to analytics stats for Android
	Android map[string]AnalyticsResponseSummaryAndroid `json:"android" api:"required"`
	// Map of region to analytics stats for iOS
	Ios map[string]AnalyticsResponseSummaryIo `json:"ios" api:"required"`
	// Map of region to analytics stats for Xcode
	Xcode map[string]AnalyticsResponseSummaryXcode `json:"xcode" api:"required"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Android     respjson.Field
		Ios         respjson.Field
		Xcode       respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSummary) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSummary) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Complete analytics for a specific region including billing breakdown
type AnalyticsResponseSummaryAndroid struct {
	// Average instance duration in minutes
	AvgDurationMinutes float64 `json:"avgDurationMinutes" api:"required"`
	// Billed minutes with platform multiplier applied
	BilledMinutes int64 `json:"billedMinutes" api:"required"`
	// Total cost in dollars
	Cost float64 `json:"cost" api:"required"`
	// Number of unique instances
	Count int64 `json:"count" api:"required"`
	// Minutes billed to credits
	CreditsBilledMinutes int64 `json:"creditsBilledMinutes" api:"required"`
	// Cost from credits (always 0)
	CreditsCost float64 `json:"creditsCost" api:"required"`
	// Minutes billed on-demand
	OnDemandBilledMinutes int64 `json:"onDemandBilledMinutes" api:"required"`
	// Cost from on-demand billing in dollars
	OnDemandCost float64 `json:"onDemandCost" api:"required"`
	// Actual runtime minutes before platform multiplier
	RuntimeMinutes int64 `json:"runtimeMinutes" api:"required"`
	// Map of subscription ID to billed minutes
	SubscriptionBilledMinutes map[string]int64 `json:"subscriptionBilledMinutes"`
	// Map of subscription ID to cost in dollars
	SubscriptionCost map[string]float64 `json:"subscriptionCost"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		AvgDurationMinutes        respjson.Field
		BilledMinutes             respjson.Field
		Cost                      respjson.Field
		Count                     respjson.Field
		CreditsBilledMinutes      respjson.Field
		CreditsCost               respjson.Field
		OnDemandBilledMinutes     respjson.Field
		OnDemandCost              respjson.Field
		RuntimeMinutes            respjson.Field
		SubscriptionBilledMinutes respjson.Field
		SubscriptionCost          respjson.Field
		ExtraFields               map[string]respjson.Field
		raw                       string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSummaryAndroid) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSummaryAndroid) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Complete analytics for a specific region including billing breakdown
type AnalyticsResponseSummaryIo struct {
	// Average instance duration in minutes
	AvgDurationMinutes float64 `json:"avgDurationMinutes" api:"required"`
	// Billed minutes with platform multiplier applied
	BilledMinutes int64 `json:"billedMinutes" api:"required"`
	// Total cost in dollars
	Cost float64 `json:"cost" api:"required"`
	// Number of unique instances
	Count int64 `json:"count" api:"required"`
	// Minutes billed to credits
	CreditsBilledMinutes int64 `json:"creditsBilledMinutes" api:"required"`
	// Cost from credits (always 0)
	CreditsCost float64 `json:"creditsCost" api:"required"`
	// Minutes billed on-demand
	OnDemandBilledMinutes int64 `json:"onDemandBilledMinutes" api:"required"`
	// Cost from on-demand billing in dollars
	OnDemandCost float64 `json:"onDemandCost" api:"required"`
	// Actual runtime minutes before platform multiplier
	RuntimeMinutes int64 `json:"runtimeMinutes" api:"required"`
	// Map of subscription ID to billed minutes
	SubscriptionBilledMinutes map[string]int64 `json:"subscriptionBilledMinutes"`
	// Map of subscription ID to cost in dollars
	SubscriptionCost map[string]float64 `json:"subscriptionCost"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		AvgDurationMinutes        respjson.Field
		BilledMinutes             respjson.Field
		Cost                      respjson.Field
		Count                     respjson.Field
		CreditsBilledMinutes      respjson.Field
		CreditsCost               respjson.Field
		OnDemandBilledMinutes     respjson.Field
		OnDemandCost              respjson.Field
		RuntimeMinutes            respjson.Field
		SubscriptionBilledMinutes respjson.Field
		SubscriptionCost          respjson.Field
		ExtraFields               map[string]respjson.Field
		raw                       string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSummaryIo) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSummaryIo) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// Complete analytics for a specific region including billing breakdown
type AnalyticsResponseSummaryXcode struct {
	// Average instance duration in minutes
	AvgDurationMinutes float64 `json:"avgDurationMinutes" api:"required"`
	// Billed minutes with platform multiplier applied
	BilledMinutes int64 `json:"billedMinutes" api:"required"`
	// Total cost in dollars
	Cost float64 `json:"cost" api:"required"`
	// Number of unique instances
	Count int64 `json:"count" api:"required"`
	// Minutes billed to credits
	CreditsBilledMinutes int64 `json:"creditsBilledMinutes" api:"required"`
	// Cost from credits (always 0)
	CreditsCost float64 `json:"creditsCost" api:"required"`
	// Minutes billed on-demand
	OnDemandBilledMinutes int64 `json:"onDemandBilledMinutes" api:"required"`
	// Cost from on-demand billing in dollars
	OnDemandCost float64 `json:"onDemandCost" api:"required"`
	// Actual runtime minutes before platform multiplier
	RuntimeMinutes int64 `json:"runtimeMinutes" api:"required"`
	// Map of subscription ID to billed minutes
	SubscriptionBilledMinutes map[string]int64 `json:"subscriptionBilledMinutes"`
	// Map of subscription ID to cost in dollars
	SubscriptionCost map[string]float64 `json:"subscriptionCost"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		AvgDurationMinutes        respjson.Field
		BilledMinutes             respjson.Field
		Cost                      respjson.Field
		Count                     respjson.Field
		CreditsBilledMinutes      respjson.Field
		CreditsCost               respjson.Field
		OnDemandBilledMinutes     respjson.Field
		OnDemandCost              respjson.Field
		RuntimeMinutes            respjson.Field
		SubscriptionBilledMinutes respjson.Field
		SubscriptionCost          respjson.Field
		ExtraFields               map[string]respjson.Field
		raw                       string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AnalyticsResponseSummaryXcode) RawJSON() string { return r.JSON.raw }
func (r *AnalyticsResponseSummaryXcode) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AnalyticsGetParams struct {
	// Start of the time range (inclusive, RFC3339)
	From time.Time `query:"from" api:"required" format:"date-time" json:"-"`
	// End of the time range (exclusive, RFC3339)
	To time.Time `query:"to" api:"required" format:"date-time" json:"-"`
	// Label selector to filter instances (e.g., "env=prod,team=backend")
	Labels param.Opt[string] `query:"labels,omitzero" json:"-"`
	// Optional region filter
	Region param.Opt[string] `query:"region,omitzero" json:"-"`
	// Optional IANA timezone used for time bucket grouping. Defaults to
	// America/Los_Angeles when omitted.
	Timezone param.Opt[string] `query:"timezone,omitzero" json:"-"`
	// Time bucket granularity for the analytics series
	//
	// Any of "hour", "day", "week", "minute".
	Bucket AnalyticsGetParamsBucket `query:"bucket,omitzero" json:"-"`
	paramObj
}

// URLQuery serializes [AnalyticsGetParams]'s query parameters as `url.Values`.
func (r AnalyticsGetParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

// Time bucket granularity for the analytics series
type AnalyticsGetParamsBucket string

const (
	AnalyticsGetParamsBucketHour   AnalyticsGetParamsBucket = "hour"
	AnalyticsGetParamsBucketDay    AnalyticsGetParamsBucket = "day"
	AnalyticsGetParamsBucketWeek   AnalyticsGetParamsBucket = "week"
	AnalyticsGetParamsBucketMinute AnalyticsGetParamsBucket = "minute"
)

type AnalyticsGetInstancesParams struct {
	// Start of the time range (inclusive, RFC3339)
	From time.Time `query:"from" api:"required" format:"date-time" json:"-"`
	// End of the time range (exclusive, RFC3339)
	To time.Time `query:"to" api:"required" format:"date-time" json:"-"`
	// Label selector to filter instances (e.g., "env=prod,team=backend")
	Labels param.Opt[string] `query:"labels,omitzero" json:"-"`
	// Optional region filter
	Region param.Opt[string] `query:"region,omitzero" json:"-"`
	// Optional IANA timezone used for minute bucket grouping. Defaults to
	// America/Los_Angeles when omitted.
	Timezone param.Opt[string] `query:"timezone,omitzero" json:"-"`
	paramObj
}

// URLQuery serializes [AnalyticsGetInstancesParams]'s query parameters as
// `url.Values`.
func (r AnalyticsGetInstancesParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}
