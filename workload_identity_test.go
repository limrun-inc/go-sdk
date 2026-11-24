package limrun_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
)

func TestFederatedClient(t *testing.T) {
	t.Setenv("LIM_API_KEY", "")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_BINDING_ID", "binding_wrong")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_TOKEN_FILE", "/does/not/exist")
	file := filepath.Join(t.TempDir(), "identity")
	os.WriteFile(file, []byte("external"), 0600)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/access_tokens" {
			if err := r.ParseForm(); err != nil {
				t.Error(err)
				return
			}
			if r.Form.Get("audience") != "binding" {
				t.Errorf("wrong binding: %s", r.Form.Get("audience"))
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "session", "token_type": "Bearer", "expires_in": 900})
			return
		}
		if r.Header.Get("Authorization") != "Bearer session" {
			t.Errorf("incorrect API credential")
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	}))
	defer server.Close()
	client := limrun.NewClient(option.WithBaseURL(server.URL), option.WithWorkloadIdentity(option.WorkloadIdentityOptions{TokenFile: file, BindingID: "binding", STSBaseURL: server.URL}))
	if _, err := client.IosInstances.List(context.Background(), limrun.IosInstanceListParams{}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("%d calls, want exchange and API call", calls)
	}
}

func TestExplicitKeyOverridesFederationDiscovery(t *testing.T) {
	t.Setenv("LIM_API_KEY", "")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_BINDING_ID", "binding_test")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_TOKEN_FILE", "/does/not/exist")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer explicit-key" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[]}`)
	}))
	defer server.Close()
	client := limrun.NewClient(option.WithAPIKey("explicit-key"), option.WithBaseURL(server.URL))
	if _, err := client.AndroidInstances.List(context.Background(), limrun.AndroidInstanceListParams{}); err != nil {
		t.Fatal(err)
	}
}

func TestFederatedClientFromTokenEnvironment(t *testing.T) {
	t.Setenv("LIM_API_KEY", "")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_TOKEN", "env-jwt")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_TOKEN_FILE", "/does/not/exist")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_BINDING_ID", "binding_env")
	var subject string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/access_tokens" {
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			subject = r.Form.Get("subject_token")
			json.NewEncoder(w).Encode(map[string]any{"access_token": "session", "token_type": "Bearer", "expires_in": 900})
			return
		}
		if r.Header.Get("Authorization") != "Bearer session" {
			t.Errorf("incorrect API credential")
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	}))
	defer server.Close()
	t.Setenv("LIMRUN_STS_BASE_URL", server.URL)
	client := limrun.NewClient(option.WithBaseURL(server.URL))
	if _, err := client.IosInstances.List(context.Background(), limrun.IosInstanceListParams{}); err != nil {
		t.Fatal(err)
	}
	if subject != "env-jwt" {
		t.Fatalf("exchanged %q, want the token variable over the file", subject)
	}
}

func TestFederatedClientFromInjectedEnvironment(t *testing.T) {
	t.Setenv("LIM_API_KEY", "")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_TOKEN", "")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_TOKEN_FILE", "")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_TOKEN_INJECTED", "true")
	t.Setenv("LIMRUN_WORKLOAD_IDENTITY_BINDING_ID", "binding_env")
	exchanged := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/access_tokens" {
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.PostForm.Has("subject_token") {
				t.Error("an injected token must not be sent by the client")
			}
			exchanged = true
			json.NewEncoder(w).Encode(map[string]any{"access_token": "session", "token_type": "Bearer", "expires_in": 900})
			return
		}
		if r.Header.Get("Authorization") != "Bearer session" {
			t.Errorf("incorrect API credential")
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	}))
	defer server.Close()
	t.Setenv("LIMRUN_STS_BASE_URL", server.URL)
	client := limrun.NewClient(option.WithBaseURL(server.URL))
	if _, err := client.IosInstances.List(context.Background(), limrun.IosInstanceListParams{}); err != nil {
		t.Fatal(err)
	}
	if !exchanged {
		t.Fatal("no exchange")
	}
}
