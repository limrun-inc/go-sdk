// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package pagination

import (
	"net/http"
	"reflect"

	"github.com/limrun-inc/go-sdk/internal/apijson"
	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
	"github.com/limrun-inc/go-sdk/packages/respjson"
)

// aliased to make [param.APIUnion] private when embedding
type paramUnion = param.APIUnion

// aliased to make [param.APIObject] private when embedding
type paramObj = param.APIObject

type Items[T any] struct {
	Items []T `json:"items"`
	// JSON contains metadata for fields, check presence with [respjson.Field.Valid].
	JSON struct {
		Items       respjson.Field
		ExtraFields map[string]respjson.Field
		raw         string
	} `json:"-"`
	cfg *requestconfig.RequestConfig
	res *http.Response
}

// Returns the unmodified JSON received from the API
func (r Items[T]) RawJSON() string { return r.JSON.raw }
func (r *Items[T]) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

// GetNextPage returns the next page as defined by this pagination style. When
// there is no next page, this function will return a 'nil' for the page value, but
// will not return an error
func (r *Items[T]) GetNextPage() (res *Items[T], err error) {
	if len(r.Items) == 0 {
		return nil, nil
	}
	items := r.Items
	if items == nil || len(items) == 0 {
		return nil, nil
	}
	cfg := r.cfg.Clone(r.cfg.Context)
	value := reflect.ValueOf(items[len(items)-1])
	field := value.FieldByName("ID")
	err = cfg.Apply(option.WithQuery("startingAfter", field.Interface().(string)))
	if err != nil {
		return nil, err
	}
	var raw *http.Response
	cfg.ResponseInto = &raw
	cfg.ResponseBodyInto = &res
	err = cfg.Execute()
	if err != nil {
		return nil, err
	}
	res.SetPageConfig(cfg, raw)
	return res, nil
}

func (r *Items[T]) SetPageConfig(cfg *requestconfig.RequestConfig, res *http.Response) {
	if r == nil {
		r = &Items[T]{}
	}
	r.cfg = cfg
	r.res = res
}

type ItemsAutoPager[T any] struct {
	page *Items[T]
	cur  T
	idx  int
	run  int
	err  error
	paramObj
}

func NewItemsAutoPager[T any](page *Items[T], err error) *ItemsAutoPager[T] {
	return &ItemsAutoPager[T]{
		page: page,
		err:  err,
	}
}

func (r *ItemsAutoPager[T]) Next() bool {
	if r.page == nil || len(r.page.Items) == 0 {
		return false
	}
	if r.idx >= len(r.page.Items) {
		r.idx = 0
		r.page, r.err = r.page.GetNextPage()
		if r.err != nil || r.page == nil || len(r.page.Items) == 0 {
			return false
		}
	}
	r.cur = r.page.Items[r.idx]
	r.run += 1
	r.idx += 1
	return true
}

func (r *ItemsAutoPager[T]) Current() T {
	return r.cur
}

func (r *ItemsAutoPager[T]) Err() error {
	return r.err
}

func (r *ItemsAutoPager[T]) Index() int {
	return r.run
}
