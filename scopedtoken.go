// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package limrun

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/limrun-inc/go-sdk/internal/apijson"
	shimjson "github.com/limrun-inc/go-sdk/internal/encoding/json"
	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
	"github.com/limrun-inc/go-sdk/packages/respjson"
)

// ScopedTokenService contains methods and other services that help with
// interacting with the limrun API.
//
// Note, unlike clients, this service does not read variables from the environment
// automatically. You should not instantiate this service directly, and instead use
// the [NewScopedTokenService] method instead.
type ScopedTokenService struct {
	Options []option.RequestOption
}

// NewScopedTokenService generates a new service that applies the given options to
// each request. These options are applied after the parent client's options (if
// there is one), and before any request-specific options.
func NewScopedTokenService(opts ...option.RequestOption) (r ScopedTokenService) {
	r = ScopedTokenService{}
	r.Options = opts
	return
}

// Mint a short-lived scoped token whose scopes limit what the holder can do, e.g.
// install a specific asset on a device through the registry. The token is verified
// offline by services holding the token signing public key and cannot be revoked,
// so keep TTLs short. It is bound to the authenticated caller's organization.
func (r *ScopedTokenService) New(ctx context.Context, body ScopedTokenNewParams, opts ...option.RequestOption) (res *ScopedToken, err error) {
	opts = slices.Concat(r.Options, opts)
	path := "v1/scoped_tokens"
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)
	return res, err
}

type ScopedToken struct {
	// The scoped token, to be sent as a Bearer token or the token query parameter.
	Token     string    `json:"token" api:"required"`
	ExpiresAt time.Time `json:"expiresAt" api:"required" format:"date-time"`
	Scopes    []string  `json:"scopes" api:"required"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Token       respjson.Field
		ExpiresAt   respjson.Field
		Scopes      respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
}

// Returns the unmodified JSON received from the API
func (r ScopedToken) RawJSON() string { return r.JSON.raw }
func (r *ScopedToken) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// The property Scopes is required.
type ScopedTokenCreateParam struct {
	// Scopes in the form <resource>:<id|_>:<action>, e.g. "device:_:install",
	// "asset:asset_01h455vb4pex5vsknk084sn02q:read" or "applerelay:\*:connect".
	// Resource IDs are the customer-visible IDs returned by the API.
	Scopes []string `json:"scopes,omitzero" api:"required"`
	// How long the token stays valid. Defaults to 3600 (1 hour), maximum is 14400 (4
	// hours).
	TtlSeconds param.Opt[int64] `json:"ttlSeconds,omitzero"`
	paramObj
}

func (r ScopedTokenCreateParam) MarshalJSON() (data []byte, err error) {
	type shadow ScopedTokenCreateParam
	return param.MarshalObject(r, (*shadow)(&r))
}
func (r *ScopedTokenCreateParam) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type ScopedTokenNewParams struct {
	ScopedTokenCreate ScopedTokenCreateParam
	paramObj
}

func (r ScopedTokenNewParams) MarshalJSON() (data []byte, err error) {
	return shimjson.Marshal(r.ScopedTokenCreate)
}
func (r *ScopedTokenNewParams) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}
