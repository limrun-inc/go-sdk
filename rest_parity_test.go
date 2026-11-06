package limrun_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/build"
	"github.com/limrun-inc/go-sdk/option"
)

func TestMalformedURLWithQueryReturnsError(t *testing.T) {
	c := limrun.NewClient(option.WithAPIKey("test"))
	err := c.Get(context.Background(), "http://localhost/%zz", limrun.IosInstanceListParams{Limit: limrun.Int(1)}, nil)
	if err == nil {
		t.Fatal("malformed URL must return an error")
	}
}

func TestInstanceUpdateClearsLabelsAndRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" || r.Header.Get("If-Match") != `"v2"` {
			t.Errorf("request=%s ETag=%s", r.Method, r.Header.Get("If-Match"))
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, ok := body["ifMatch"]; ok {
			t.Error("ifMatch leaked into body")
		}
		if string(body["metadata"]) != `{"labels":{}}` || string(body["spec"]) != `{"role":{"permissions":[]}}` {
			t.Errorf("clear payload=%v", body)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := limrun.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	err := c.IosInstances.Update(context.Background(), "ios_test", limrun.IosInstanceUpdateParams{IfMatch: `"v2"`, Metadata: limrun.IosInstanceUpdateParamsMetadata{Labels: map[string]string{}}, Spec: limrun.IosInstanceUpdateParamsSpec{Role: limrun.IosInstanceUpdateParamsSpecRole{Permissions: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
}
func TestInstancePaginationUsesMetadataID(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			w.Write([]byte(`[{"metadata":{"id":"ios_last"}}]`))
		} else {
			if r.URL.Query().Get("startingAfter") != "ios_last" {
				t.Errorf("cursor=%s", r.URL.RawQuery)
			}
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	c := limrun.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	pages := c.IosInstances.ListAutoPaging(context.Background(), limrun.IosInstanceListParams{})
	for pages.Next() {
	}
	if pages.Err() != nil {
		t.Fatal(pages.Err())
	}
	if count != 2 {
		t.Errorf("calls=%d", count)
	}
}

func TestAnalyticsOptionalBoundsAndDeepFilters(t *testing.T) {
	for _, query := range []interface{ URLQuery() (url.Values, error) }{
		limrun.AnalyticsGetParams{StartedAt: limrun.AnalyticsTimeFilter{Gte: limrun.String("2026-11-01T00:00:00Z")}, StoppedAt: limrun.AnalyticsTimeFilter{Lte: limrun.String("2026-11-05T00:00:00Z")}},
		limrun.AnalyticsGetInstancesParams{StartedAt: limrun.AnalyticsTimeFilter{Gte: limrun.String("2026-11-01T00:00:00Z")}, StoppedAt: limrun.AnalyticsTimeFilter{Lte: limrun.String("2026-11-05T00:00:00Z")}},
	} {
		q, err := query.URLQuery()
		if err != nil {
			t.Fatal(err)
		}
		if q.Get("startedAt[gte]") != "2026-11-01T00:00:00Z" || q.Get("stoppedAt[lte]") != "2026-11-05T00:00:00Z" || q.Has("from") || q.Has("to") {
			t.Fatalf("query=%s", q.Encode())
		}
	}
}

func TestGetOrUploadMetadataProgressAndMD5(t *testing.T) {
	data := []byte("a simulator app bundle")
	path := filepath.Join(t.TempDir(), "app.zip")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	checksum := fmt.Sprintf("%x", md5.Sum(data))
	creates, uploads, progress := 0, 0, 0
	var uploaded, total int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/upload" {
			uploads++
			if r.Method != "PUT" || r.ContentLength != int64(len(data)) || r.Header.Get("Content-Type") != "application/octet-stream" {
				t.Errorf("upload=%s length=%d", r.Method, r.ContentLength)
			}
			got, _ := io.ReadAll(r.Body)
			if !bytes.Equal(got, data) {
				t.Errorf("uploaded=%q", got)
			}
			return
		}
		creates++
		var params map[string]any
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			t.Error(err)
		}
		if params["kind"] != "App" || params["platform"] != "ios" || params["ttl"] != "24h" || params["bundleIdentifier"] != "com.example.app" || params["buildVersion"] != "42" {
			t.Errorf("metadata=%v", params)
		}
		serverMD5 := ""
		if creates > 1 {
			serverMD5 = checksum
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "asset_test", "name": "app.zip", "kind": "App", "platform": "ios", "signedUploadUrl": server.URL + "/upload", "signedDownloadUrl": server.URL + "/download", "md5": serverMD5})
	}))
	defer server.Close()
	client := limrun.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"))
	bundle, version := "com.example.app", "42"
	params := limrun.AssetGetOrUploadParams{Path: path, Ttl: limrun.String("24h"), Platform: limrun.AssetGetOrNewParamsPlatformIos, UploadOptions: &limrun.AssetUploadOptions{BundleIdentifier: &bundle, BuildVersion: &version}, OnUploadProgress: func(n, size int64) { progress++; uploaded, total = n, size }}
	for i := 0; i < 2; i++ {
		res, err := client.Assets.GetOrUpload(context.Background(), params)
		if err != nil {
			t.Fatal(err)
		}
		if res.Md5 != checksum {
			t.Fatalf("md5=%s", res.Md5)
		}
	}
	if uploads != 1 || progress == 0 || uploaded != int64(len(data)) || total != int64(len(data)) {
		t.Fatalf("uploads=%d progress=%d bytes=%d/%d", uploads, progress, uploaded, total)
	}
}

func TestDirectInstanceErrorsMatchRESTNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, "instance gone")
	}))
	defer server.Close()
	process := build.Exec(context.Background(), build.RunExecRequest{Command: "run", CommandLine: "pwd"}, build.ExecOptions{APIURL: server.URL, Token: "test"})
	_, err := process.Wait(context.Background())
	var rest *limrun.Error
	var direct *limrun.DirectInstanceHTTPError
	if !errors.As(err, &rest) || rest.StatusCode != 404 || !errors.As(err, &direct) || direct.Body != "instance gone" || !limrun.IsDirectInstanceHTTPError(err, 404) {
		t.Fatalf("error=%T %v", err, err)
	}
	if rest.Error() == "" {
		t.Error("missing REST message")
	}
}
