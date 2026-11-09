package limrun

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/limrun-inc/go-sdk/internal/apijson"
	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
	"github.com/limrun-inc/go-sdk/packages/respjson"
)

// AccessTokenService mints and revokes temporary credentials whose permissions
// are a subset of the caller's.
//
// Note, unlike clients, this service does not read variables from the environment
// automatically. You should not instantiate this service directly, and instead use
// the [NewAccessTokenService] method instead.
type AccessTokenService struct {
	Options []option.RequestOption
}

// NewAccessTokenService generates a new service that applies the given options to
// each request. These options are applied after the parent client's options (if
// there is one), and before any request-specific options.
func NewAccessTokenService(opts ...option.RequestOption) (r AccessTokenService) {
	r = AccessTokenService{}
	r.Options = opts
	return
}

// Mint a temporary credential whose permissions are covered by the caller.
func (r *AccessTokenService) New(ctx context.Context, body AccessTokenNewParams, opts ...option.RequestOption) (res *AccessToken, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/access_tokens"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)
	return
}

// Revoke a credential by the ID returned when it was minted.
func (r *AccessTokenService) Revoke(ctx context.Context, id string, opts ...option.RequestOption) (res *AccessTokenRevokeResponse, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/access_tokens/" + url.PathEscape(id)
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, &res, opts...)
	return
}

type AccessToken struct {
	// Credential ID, used to revoke the token.
	ID string `json:"id" api:"required"`
	// The access token, to be sent as a Bearer token.
	Token string `json:"token" api:"required"`
	// Permissions granted to this credential.
	Permissions []string `json:"permissions"`
	// Deprecated: use Permissions.
	Scopes    []string  `json:"scopes" api:"required"`
	ExpiresAt time.Time `json:"expiresAt" api:"required" format:"date-time"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		ID          respjson.Field
		Token       respjson.Field
		Permissions respjson.Field
		Scopes      respjson.Field
		ExpiresAt   respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AccessToken) RawJSON() string { return r.JSON.raw }
func (r *AccessToken) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AccessTokenNewParams struct {
	// Permissions in the form `<resource>:<id|*>:<action>`, e.g. `device:*:install`,
	// `asset:asset_01h455vb4pex5vsknk084sn02q:read` or `applerelay:*:connect`.
	// Instance control accepts `ios:<id>:all`, `android:<id>:all`, `xcode:<id>:all`,
	// and `gradle:<id>:all`. Use the returned instance ID, including `sandbox_` for
	// Xcode. iOS and Android also accept `ios:<id>:stream` and `android:<id>:stream`
	// for interactive streaming without device API or app-management access.
	// Instance wildcards and other instance actions are not accepted. See
	// https://docs.limrun.com/docs/reference/permissions for targets and actions.
	Permissions []string `json:"permissions,omitzero"`
	// Deprecated: use Permissions. If both are supplied, their permission sets must
	// match.
	Scopes []string `json:"scopes,omitzero"`
	// How long the token stays valid. Defaults to 3600 (1 hour), maximum is 14400 (4
	// hours).
	TtlSeconds param.Opt[int64] `json:"ttlSeconds,omitzero"`
	paramObj
}

func (r AccessTokenNewParams) MarshalJSON() (data []byte, err error) {
	type shadow AccessTokenNewParams
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *AccessTokenNewParams) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type AccessTokenRevokeResponse struct {
	Status string `json:"status" api:"required"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Status      respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r AccessTokenRevokeResponse) RawJSON() string { return r.JSON.raw }
func (r *AccessTokenRevokeResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}
