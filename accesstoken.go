package limrun

import (
	"context"
	"net/http"
	"net/url"
	"slices"

	"github.com/limrun-inc/go-sdk/internal/apijson"
	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
	"github.com/limrun-inc/go-sdk/packages/respjson"
)

type AccessToken struct {
	ID          string   `json:"id"`
	Token       string   `json:"token"`
	Permissions []string `json:"permissions"`
	Scopes      []string `json:"scopes"`
	ExpiresAt   string   `json:"expiresAt"`
	JSON        struct {
		ID          respjson.Field
		Token       respjson.Field
		Permissions respjson.Field
		Scopes      respjson.Field
		ExpiresAt   respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

func (r AccessToken) RawJSON() string                  { return r.JSON.raw }
func (r *AccessToken) UnmarshalJSON(data []byte) error { return apijson.UnmarshalRoot(data, r) }

type AccessTokenNewParams struct {
	// Permissions in the form `<resource>:<id|*>:<action>`, e.g. `device:*:install`,
	// `asset:asset_01h455vb4pex5vsknk084sn02q:read` or `applerelay:*:connect`.
	// Instance control accepts `ios:<id>:all`, `android:<id>:all`, `xcode:<id>:all`,
	// and `gradle:<id>:all`. Use the returned instance ID, including `sandbox_` for Xcode.
	// iOS and Android also accept `ios:<id>:stream` and `android:<id>:stream` for
	// interactive streaming without device API or app-management access.
	// Instance wildcards and other instance actions are not accepted.
	// See https://docs.limrun.com/docs/reference/permissions for targets and actions.
	Permissions []string `json:"permissions,omitzero"`
	Scopes      []string `json:"scopes,omitzero"`
	// How long the token stays valid. Defaults to 3600 (1 hour), maximum is 14400 (4
	// hours).
	TtlSeconds param.Opt[float64] `json:"ttlSeconds,omitzero"`
	paramObj
}

func (r AccessTokenNewParams) MarshalJSON() ([]byte, error) {
	type shadow AccessTokenNewParams
	return param.MarshalObject(r, (*shadow)(&r))
}

type AccessTokenRevokeResponse struct {
	Status string `json:"status"`
	JSON   struct {
		Status      respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

func (r AccessTokenRevokeResponse) RawJSON() string { return r.JSON.raw }
func (r *AccessTokenRevokeResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AccessTokenService struct{ Options []option.RequestOption }

func NewAccessTokenService(opts ...option.RequestOption) AccessTokenService {
	return AccessTokenService{Options: opts}
}
func (r *AccessTokenService) New(ctx context.Context, params AccessTokenNewParams, opts ...option.RequestOption) (res *AccessToken, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPost, "v1/access_tokens", params, &res, slices.Concat(r.Options, opts)...)
	return
}
func (r *AccessTokenService) Revoke(ctx context.Context, id string, opts ...option.RequestOption) (res *AccessTokenRevokeResponse, err error) {
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, "v1/access_tokens/"+url.PathEscape(id), nil, &res, slices.Concat(r.Options, opts)...)
	return
}
