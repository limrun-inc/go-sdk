// Hand-written following the Stainless resource pattern; a future generation
// from the OpenAPI spec reconciles with this file.

package limrun_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/internal/testutil"
	"github.com/limrun-inc/go-sdk/option"
)

func TestMacosInstanceNewWithOptionalParams(t *testing.T) {
	baseURL := "http://localhost:4010"
	if envURL, ok := os.LookupEnv("TEST_API_BASE_URL"); ok {
		baseURL = envURL
	}
	if !testutil.CheckTestServer(t, baseURL) {
		return
	}
	client := limrun.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("My API Key"),
	)
	_, err := client.MacosInstances.New(context.TODO(), limrun.MacosInstanceNewParams{
		ReuseIfExists: limrun.Bool(true),
		Wait:          limrun.Bool(true),
		Metadata: limrun.MacosInstanceNewParamsMetadata{
			DisplayName: limrun.String("displayName"),
			Labels: map[string]string{
				"foo": "string",
			},
			Name: limrun.String("u/u123/preview"),
		},
		Spec: limrun.MacosInstanceNewParamsSpec{
			Clues: []limrun.MacosInstanceNewParamsSpecClue{{
				Kind:     "ClientIP",
				ClientIP: limrun.String("clientIp"),
			}},
			HardTimeout:       limrun.String("hardTimeout"),
			InactivityTimeout: limrun.String("inactivityTimeout"),
			Jurisdiction:      "us",
			Role: limrun.MacosInstanceNewParamsSpecRole{
				Permissions: []string{"string"},
			},
		},
	})
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestMacosInstanceListWithOptionalParams(t *testing.T) {
	baseURL := "http://localhost:4010"
	if envURL, ok := os.LookupEnv("TEST_API_BASE_URL"); ok {
		baseURL = envURL
	}
	if !testutil.CheckTestServer(t, baseURL) {
		return
	}
	client := limrun.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("My API Key"),
	)
	_, err := client.MacosInstances.List(context.TODO(), limrun.MacosInstanceListParams{
		EndingBefore:  limrun.String("endingBefore"),
		LabelSelector: limrun.String("env=prod,version=1.2"),
		Limit:         limrun.Int(50),
		StartingAfter: limrun.String("startingAfter"),
		State:         limrun.String("assigned,ready"),
	})
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestMacosInstanceDelete(t *testing.T) {
	baseURL := "http://localhost:4010"
	if envURL, ok := os.LookupEnv("TEST_API_BASE_URL"); ok {
		baseURL = envURL
	}
	if !testutil.CheckTestServer(t, baseURL) {
		return
	}
	client := limrun.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("My API Key"),
	)
	err := client.MacosInstances.Delete(context.TODO(), "id")
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestMacosInstanceGet(t *testing.T) {
	baseURL := "http://localhost:4010"
	if envURL, ok := os.LookupEnv("TEST_API_BASE_URL"); ok {
		baseURL = envURL
	}
	if !testutil.CheckTestServer(t, baseURL) {
		return
	}
	client := limrun.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("My API Key"),
	)
	_, err := client.MacosInstances.Get(context.TODO(), "id")
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestMacosInstanceUpdate(t *testing.T) {
	baseURL := "http://localhost:4010"
	if envURL, ok := os.LookupEnv("TEST_API_BASE_URL"); ok {
		baseURL = envURL
	}
	if !testutil.CheckTestServer(t, baseURL) {
		return
	}
	client := limrun.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("My API Key"),
	)
	err := client.MacosInstances.Update(context.TODO(), "id", limrun.MacosInstanceUpdateParams{
		IfMatch: `"v1"`,
		Metadata: limrun.MacosInstanceUpdateParamsMetadata{
			Labels: map[string]string{"foo": "string"},
		},
		Spec: limrun.MacosInstanceUpdateParamsSpec{
			Role: limrun.MacosInstanceUpdateParamsSpecRole{Permissions: []string{"string"}},
		},
	})
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

// TestMacosInstanceRoutes checks the routes, query parameters, list cursor and
// conditional update against a local server, like the TypeScript SDK's test.
func TestMacosInstanceRoutes(t *testing.T) {
	const instance = `{"metadata":{"id":"macos_eu_01h","organizationId":"org","createdAt":"2026-11-23T00:00:00Z","name":"u/u123/preview"},` +
		`"spec":{"region":"eu-north1","inactivityTimeout":"3m"},` +
		`"status":{"state":"ready","endpointWebSocketUrl":"wss://example.test/v1/macos_eu_01h/endpointWebSocket","streamToken":"stream"}}`
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		q := r.URL.Query()
		switch {
		case r.Method == http.MethodPost:
			if q.Get("wait") != "true" {
				t.Errorf("create query=%s", r.URL.RawQuery)
			}
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if string(body["metadata"]) != `{"labels":{"env":"test"}}` || string(body["spec"]) != `{"inactivityTimeout":"5m","jurisdiction":"eu"}` {
				t.Errorf("create body=%v", body)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/v1/macos_instances":
			if q.Get("state") != "ready" || q.Get("labelSelector") != "env=test" {
				t.Errorf("list query=%s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			if q.Has("startingAfter") {
				if q.Get("startingAfter") != "macos_eu_01h" {
					t.Errorf("cursor=%s", r.URL.RawQuery)
				}
				w.Write([]byte(`[]`))
			} else {
				w.Write([]byte("[" + instance + "]"))
			}
			return
		case r.Method == http.MethodPatch:
			if r.Header.Get("If-Match") != `"v1"` {
				t.Errorf("If-Match=%s", r.Header.Get("If-Match"))
			}
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if len(body) != 1 || string(body["metadata"]) != `{"labels":{}}` {
				t.Errorf("update body=%v", body)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodDelete:
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(instance))
	}))
	defer srv.Close()
	client := limrun.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	ctx := context.Background()

	created, err := client.MacosInstances.New(ctx, limrun.MacosInstanceNewParams{
		Wait:     limrun.Bool(true),
		Metadata: limrun.MacosInstanceNewParamsMetadata{Labels: map[string]string{"env": "test"}},
		Spec:     limrun.MacosInstanceNewParamsSpec{InactivityTimeout: limrun.String("5m"), Jurisdiction: "eu"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status.EndpointWebSocketURL != "wss://example.test/v1/macos_eu_01h/endpointWebSocket" || created.Status.StreamToken != "stream" || created.Metadata.Name != "u/u123/preview" {
		t.Errorf("created=%s", created.RawJSON())
	}

	var listed []string
	pages := client.MacosInstances.ListAutoPaging(ctx, limrun.MacosInstanceListParams{State: limrun.String("ready"), LabelSelector: limrun.String("env=test")})
	for pages.Next() {
		listed = append(listed, pages.Current().Metadata.ID)
	}
	if err := pages.Err(); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0] != "macos_eu_01h" {
		t.Errorf("listed=%v", listed)
	}

	if _, err := client.MacosInstances.Get(ctx, "macos_eu_01h"); err != nil {
		t.Fatal(err)
	}
	if err := client.MacosInstances.Update(ctx, "macos_eu_01h", limrun.MacosInstanceUpdateParams{IfMatch: `"v1"`, Metadata: limrun.MacosInstanceUpdateParamsMetadata{Labels: map[string]string{}}}); err != nil {
		t.Fatal(err)
	}
	if err := client.MacosInstances.Delete(ctx, "macos_eu_01h"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"POST /v1/macos_instances",
		"GET /v1/macos_instances",
		"GET /v1/macos_instances",
		"GET /v1/macos_instances/macos_eu_01h",
		"PATCH /v1/macos_instances/macos_eu_01h",
		"DELETE /v1/macos_instances/macos_eu_01h",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls=%v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d=%s, want %s", i, calls[i], want[i])
		}
	}
}
