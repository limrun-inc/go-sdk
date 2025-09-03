package api

import (
	"net/http"
)

func WithToken(token string) ClientOption {
	return WithClient(&http.Client{
		Transport: &authTransport{
			token: token,
			base:  http.DefaultTransport,
		},
	})
}

type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}
