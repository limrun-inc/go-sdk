package direct

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/limrun-inc/go-sdk/internal/apierror"
	"github.com/limrun-inc/go-sdk/internal/proxy"
)

type HTTPError struct {
	Status    int
	Body      string
	Operation string
	Header    http.Header
	Request   *http.Request
	Response  *http.Response
}

func (e *HTTPError) Error() string {
	message := fmt.Sprintf("%s failed: %d", e.Operation, e.Status)
	if e.Body != "" {
		message += " " + e.Body
	}
	return message
}

// As preserves the REST client's status-based error handling for a missing instance.
func (e *HTTPError) As(target any) bool {
	if out, ok := target.(**apierror.Error); ok && e.Status == http.StatusNotFound {
		req := e.Request
		if req == nil {
			req, _ = http.NewRequest(http.MethodGet, "http://instance", nil)
		}
		res := e.Response
		if res == nil {
			res = &http.Response{StatusCode: e.Status, Header: e.Header, Request: req}
		}
		value := &apierror.Error{StatusCode: e.Status, Request: req, Response: res}
		body, _ := json.Marshal(map[string]string{"message": e.Error()})
		_ = value.UnmarshalJSON(body)
		*out = value
		return true
	}
	return false
}
func IsHTTPError(err error, status int) bool {
	var direct *HTTPError
	if errors.As(err, &direct) {
		return direct.Status == status
	}
	var api *apierror.Error
	return errors.As(err, &api) && api.StatusCode == status
}
func ResponseError(operation string, res *http.Response, body string) *HTTPError {
	return &HTTPError{Status: res.StatusCode, Body: body, Operation: operation, Header: res.Header, Request: res.Request, Response: res}
}

type Client struct {
	URL, Token string
	HTTPClient *http.Client
}

func (c Client) Do(ctx context.Context, method, url string, body io.Reader, contentType string, length int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if length >= 0 {
		req.ContentLength = length
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = proxy.Client()
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return nil, ResponseError(method+" "+req.URL.Path, res, string(b))
	}
	return res, nil
}
func (c Client) JSON(ctx context.Context, method, path string, params, result any) error {
	var body io.Reader
	ct := ""
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
		ct = "application/json"
	}
	url := strings.TrimRight(c.URL, "/") + "/" + strings.TrimLeft(path, "/")
	res, err := c.Do(ctx, method, url, body, ct, -1)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if result != nil {
		return json.NewDecoder(res.Body).Decode(result)
	}
	return nil
}
func (c Client) Download(ctx context.Context, url, path string) error {
	res, err := c.Do(ctx, http.MethodGet, url, nil, "", -1)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, res.Body)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func (c Client) Upload(ctx context.Context, url, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return "", err
	}
	res, err := c.Do(ctx, http.MethodPut, url, f, "application/octet-stream", stat.Size())
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var result struct {
		Path string `json:"path"`
	}
	err = json.NewDecoder(res.Body).Decode(&result)
	return result.Path, err
}

type TrustedCACertificate struct {
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
}

func (c Client) AddCACertificate(ctx context.Context, pem string) (*TrustedCACertificate, error) {
	res, err := c.Do(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/trusted-cas", strings.NewReader(pem), "application/x-pem-file", -1)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var result TrustedCACertificate
	err = json.NewDecoder(res.Body).Decode(&result)
	return &result, err
}
