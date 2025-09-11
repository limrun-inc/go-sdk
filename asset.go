// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package limrun

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

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
	opts = append(r.Options[:], opts...)
	path := "v1/assets"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, query, &res, opts...)
	return
}

// Get the asset with given ID.
func (r *AssetService) Get(ctx context.Context, assetID string, query AssetGetParams, opts ...option.RequestOption) (res *Asset, err error) {
	opts = append(r.Options[:], opts...)
	if assetID == "" {
		err = errors.New("missing required assetId parameter")
		return
	}
	path := fmt.Sprintf("v1/assets/%s", assetID)
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, query, &res, opts...)
	return
}

// Creates an asset and returns upload and download URLs. If there is a
// corresponding file uploaded in the storage with given name, its MD5 is returned
// so you can check if a re-upload is necessary. If no MD5 is returned, then there
// is no corresponding file in the storage so downloading it directly or using it
// in instances will fail until you use the returned upload URL to submit the file.
func (r *AssetService) GetOrNew(ctx context.Context, body AssetGetOrNewParams, opts ...option.RequestOption) (res *AssetGetOrNewResponse, err error) {
	opts = append(r.Options[:], opts...)
	path := "v1/assets"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPut, path, body, &res, opts...)
	return
}

type Asset struct {
	ID   string `json:"id,required"`
	Name string `json:"name,required"`
	// Returned only if there is a corresponding file uploaded already.
	Md5               string `json:"md5"`
	SignedDownloadURL string `json:"signedDownloadUrl"`
	SignedUploadURL   string `json:"signedUploadUrl"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		ID                respjson.Field
		Name              respjson.Field
		Md5               respjson.Field
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

type AssetGetOrNewResponse struct {
	ID                string `json:"id,required"`
	Name              string `json:"name,required"`
	SignedDownloadURL string `json:"signedDownloadUrl,required"`
	SignedUploadURL   string `json:"signedUploadUrl,required"`
	// Returned only if there is a corresponding file uploaded already.
	Md5 string `json:"md5"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		ID                respjson.Field
		Name              respjson.Field
		SignedDownloadURL respjson.Field
		SignedUploadURL   respjson.Field
		Md5               respjson.Field
		ExtraFields       map[string]respjson.Field
		raw               string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AssetGetOrNewResponse) RawJSON() string { return r.JSON.raw }
func (r *AssetGetOrNewResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AssetListParams struct {
	// Toggles whether a download URL should be included in the response
	IncludeDownloadURL param.Opt[bool] `query:"includeDownloadUrl,omitzero" json:"-"`
	// Toggles whether an upload URL should be included in the response
	IncludeUploadURL param.Opt[bool] `query:"includeUploadUrl,omitzero" json:"-"`
	// Query by file name
	NameFilter param.Opt[string] `query:"nameFilter,omitzero" json:"-"`
	paramObj
}

// URLQuery serializes [AssetListParams]'s query parameters as `url.Values`.
func (r AssetListParams) URLQuery() (v url.Values, err error) {
	return apiquery.MarshalWithSettings(r, apiquery.QuerySettings{
		ArrayFormat:  apiquery.ArrayQueryFormatComma,
		NestedFormat: apiquery.NestedQueryFormatBrackets,
	})
}

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
	Name string `json:"name,required"`
	paramObj
}

func (r AssetGetOrNewParams) MarshalJSON() (data []byte, err error) {
	type shadow AssetGetOrNewParams
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AssetGetOrNewParams) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}
