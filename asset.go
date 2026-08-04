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

// AssetService contains methods and other services that help with interacting with
// the limrun API.
//
// Note, unlike clients, this service does not read variables from the environment
// automatically. You should not instantiate this service directly, and instead use
// the [NewAssetService] method instead.
type AssetService struct {
	Options []option.RequestOption
}

// NewAssetService generates a new service that applies the given options to each
// request. These options are applied after the parent client's options (if there
// is one), and before any request-specific options.
func NewAssetService(opts ...option.RequestOption) (r AssetService) {
	r = AssetService{}
	r.Options = opts
	return
}

// List organization's all assets with given filters. If none given, return all
// assets.
func (r *AssetService) List(ctx context.Context, query AssetListParams, opts ...option.RequestOption) (res *[]Asset, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/assets"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, query, &res, opts...)
	return res, err
}

// Delete the asset with given ID.
func (r *AssetService) Delete(ctx context.Context, assetID string, opts ...option.RequestOption) (err error) {
	opts = slices.Concat(r.Options, opts)
	opts = append([]option.RequestOption{option.WithHeader("Accept", "*/*")}, opts...)
	if assetID == "" {
		err = errors.New("missing required assetId parameter")
		return err
	}
	path := fmt.Sprintf("v1/assets/%s", assetID)
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)
	return err
}

// Get the asset with given ID.
func (r *AssetService) Get(ctx context.Context, assetID string, query AssetGetParams, opts ...option.RequestOption) (res *Asset, err error) {
	opts = slices.Concat(r.Options, opts)
	if assetID == "" {
		err = errors.New("missing required assetId parameter")
		return nil, err
	}
	path := fmt.Sprintf("v1/assets/%s", assetID)
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, query, &res, opts...)
	return res, err
}

// Creates an asset and returns upload and download URLs. If there is a
// corresponding file uploaded in the storage with given name, its MD5 is returned
// so you can check if a re-upload is necessary. If no MD5 is returned, then there
// is no corresponding file in the storage so downloading it directly or using it
// in instances will fail until you use the returned upload URL to submit the file.
func (r *AssetService) GetOrNew(ctx context.Context, body AssetGetOrNewParams, opts ...option.RequestOption) (res *AssetGetOrNewResponse, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/assets"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPut, path, body, &res, opts...)
	return res, err
}

type Asset struct {
	ID string `json:"id" api:"required"`
	// Any of "App", "Keychain".
	Kind AssetKind `json:"kind" api:"required"`
	Name string    `json:"name" api:"required"`
	// Human-readable display name for the asset. If not set, the name should be used.
	DisplayName string `json:"displayName"`
	// When set, the time after which the asset is automatically deleted.
	ExpiresAt time.Time `json:"expiresAt" format:"date-time"`
	// Returned only if there is a corresponding file uploaded already.
	Md5 string `json:"md5"`
	// Deprecated: alias of platform, always mirrors it. Use platform instead.
	//
	// Any of "ios", "android", "xcode".
	//
	// Deprecated: deprecated
	Os AssetOs `json:"os"`
	// The platform this asset is for. If not set, the asset is available for all
	// platforms.
	//
	// Any of "ios", "android", "xcode".
	Platform          AssetPlatform `json:"platform"`
	SignedDownloadURL string        `json:"signedDownloadUrl"`
	SignedUploadURL   string        `json:"signedUploadUrl"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		ID                respjson.Field
		Kind              respjson.Field
		Name              respjson.Field
		DisplayName       respjson.Field
		ExpiresAt         respjson.Field
		Md5               respjson.Field
		Os                respjson.Field
		Platform          respjson.Field
		SignedDownloadURL respjson.Field
		SignedUploadURL   respjson.Field
		ExtraFields       map[string]respjson.Field
		raw               string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r Asset) RawJSON() string { return r.JSON.raw }
func (r *Asset) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AssetKind string

const (
	AssetKindApp      AssetKind = "App"
	AssetKindKeychain AssetKind = "Keychain"
)

// Deprecated: alias of platform, always mirrors it. Use platform instead.
type AssetOs string

const (
	AssetOsIos     AssetOs = "ios"
	AssetOsAndroid AssetOs = "android"
	AssetOsXcode   AssetOs = "xcode"
)

// The platform this asset is for. If not set, the asset is available for all
// platforms.
type AssetPlatform string

const (
	AssetPlatformIos     AssetPlatform = "ios"
	AssetPlatformAndroid AssetPlatform = "android"
	AssetPlatformXcode   AssetPlatform = "xcode"
)

type AssetGetOrNewResponse struct {
	ID string `json:"id" api:"required"`
	// Any of "App", "Keychain".
	Kind              AssetGetOrNewResponseKind `json:"kind" api:"required"`
	Name              string                    `json:"name" api:"required"`
	SignedDownloadURL string                    `json:"signedDownloadUrl" api:"required"`
	SignedUploadURL   string                    `json:"signedUploadUrl" api:"required"`
	// When set, the time after which the asset is automatically deleted.
	ExpiresAt time.Time `json:"expiresAt" format:"date-time"`
	// Returned only if there is a corresponding file uploaded already.
	Md5 string `json:"md5"`
	// Any of "ios", "android", "xcode".
	Platform AssetGetOrNewResponsePlatform `json:"platform"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		ID                respjson.Field
		Kind              respjson.Field
		Name              respjson.Field
		SignedDownloadURL respjson.Field
		SignedUploadURL   respjson.Field
		ExpiresAt         respjson.Field
		Md5               respjson.Field
		Platform          respjson.Field
		ExtraFields       map[string]respjson.Field
		raw               string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AssetGetOrNewResponse) RawJSON() string { return r.JSON.raw }
func (r *AssetGetOrNewResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AssetGetOrNewResponseKind string

const (
	AssetGetOrNewResponseKindApp      AssetGetOrNewResponseKind = "App"
	AssetGetOrNewResponseKindKeychain AssetGetOrNewResponseKind = "Keychain"
)

type AssetGetOrNewResponsePlatform string

const (
	AssetGetOrNewResponsePlatformIos     AssetGetOrNewResponsePlatform = "ios"
	AssetGetOrNewResponsePlatformAndroid AssetGetOrNewResponsePlatform = "android"
	AssetGetOrNewResponsePlatformXcode   AssetGetOrNewResponsePlatform = "xcode"
)

type AssetListParams struct {
	// If true, also includes assets from Limrun App Store where you have access to.
	// App Store assets will be returned with a "appstore/" prefix in their names.
	IncludeAppStore param.Opt[bool] `query:"includeAppStore,omitzero" json:"-"`
	// Toggles whether a download URL should be included in the response
	IncludeDownloadURL param.Opt[bool] `query:"includeDownloadUrl,omitzero" json:"-"`
	// Toggles whether an upload URL should be included in the response
	IncludeUploadURL param.Opt[bool] `query:"includeUploadUrl,omitzero" json:"-"`
	// Maximum number of items to be returned. The default is 50.
	Limit param.Opt[int64] `query:"limit,omitzero" json:"-"`
	// Case-sensitive exact match on the asset name. Cannot be combined with
	// namePrefixFilter. When combined with includeAppStore=true, a leading "appstore/"
	// is stripped before querying App Store assets (whose stored names never carry the
	// prefix).
	NameFilter param.Opt[string] `query:"nameFilter,omitzero" json:"-"`
	// Case-sensitive prefix match on the asset name. LIKE wildcards ("%", "\_") in the
	// value are treated as literal characters, not wildcards. Empty string is rejected
	// with 400; omit the parameter if no filtering is desired. Cannot be combined with
	// nameFilter. When combined with includeAppStore=true, a leading "appstore/" is
	// stripped before querying App Store assets (whose stored names never carry the
	// prefix); a partial prefix like "appstor" will not match any App Store assets.
	NamePrefixFilter param.Opt[string] `query:"namePrefixFilter,omitzero" json:"-"`
	// Filters assets by kind.
	//
	// Any of "App", "Keychain".
	KindFilter AssetListParamsKindFilter `query:"kindFilter,omitzero" json:"-"`
	paramObj
}

// URLQuery serializes [AssetListParams]'s query parameters as `url.Values`.
func (r AssetListParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

// Filters assets by kind.
type AssetListParamsKindFilter string

const (
	AssetListParamsKindFilterApp      AssetListParamsKindFilter = "App"
	AssetListParamsKindFilterKeychain AssetListParamsKindFilter = "Keychain"
)

type AssetGetParams struct {
	// Toggles whether a download URL should be included in the response
	IncludeDownloadURL param.Opt[bool] `query:"includeDownloadUrl,omitzero" json:"-"`
	// Toggles whether an upload URL should be included in the response
	IncludeUploadURL param.Opt[bool] `query:"includeUploadUrl,omitzero" json:"-"`
	paramObj
}

// URLQuery serializes [AssetGetParams]'s query parameters as `url.Values`.
func (r AssetGetParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

type AssetGetOrNewParams struct {
	Name string `json:"name" api:"required"`
	// Optional time-to-live as a Go duration string (e.g. "24h"). When set, the asset
	// is deleted this long after now; minimum is 1m. Omit for no expiry. On re-upload
	// of an existing asset, a value updates the expiry while omitting it leaves the
	// current expiry unchanged.
	Ttl param.Opt[string] `json:"ttl,omitzero"`
	// Any of "App", "Keychain".
	Kind AssetGetOrNewParamsKind `json:"kind,omitzero"`
	// Any of "ios", "android", "xcode".
	Platform AssetGetOrNewParamsPlatform `json:"platform,omitzero"`
	paramObj
}

func (r AssetGetOrNewParams) MarshalJSON() (data []byte, err error) {
	type shadow AssetGetOrNewParams
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AssetGetOrNewParams) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AssetGetOrNewParamsKind string

const (
	AssetGetOrNewParamsKindApp      AssetGetOrNewParamsKind = "App"
	AssetGetOrNewParamsKindKeychain AssetGetOrNewParamsKind = "Keychain"
)

type AssetGetOrNewParamsPlatform string

const (
	AssetGetOrNewParamsPlatformIos     AssetGetOrNewParamsPlatform = "ios"
	AssetGetOrNewParamsPlatformAndroid AssetGetOrNewParamsPlatform = "android"
	AssetGetOrNewParamsPlatformXcode   AssetGetOrNewParamsPlatform = "xcode"
)
