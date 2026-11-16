package option

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkloadIdentityRenewal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var mu sync.Mutex
	var identities []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/access_tokens" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected exchange request")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("audience") != "binding_customer" || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:token-exchange" {
			t.Error("invalid form")
		}
		mu.Lock()
		identities = append(identities, r.Form.Get("subject_token"))
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"access_token": "limrun-session", "token_type": "Bearer", "expires_in": 900})
	}))
	defer server.Close()
	c := &workloadCredentials{options: WorkloadIdentityOptions{TokenFile: path, BindingID: "binding_customer", STSBaseURL: server.URL}, client: server.Client()}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := c.get(context.Background())
			if err != nil || token != "limrun-session" {
				t.Errorf("get: %s %v", token, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("%d exchanges, want 1", calls.Load())
	}
	if err := os.WriteFile(path, []byte("rotated"), 0600); err != nil {
		t.Fatal(err)
	}
	c.refreshAt = time.Now().Add(-time.Second)
	if _, err := c.get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || identities[1] != "rotated" {
		t.Fatalf("identity was not reread: %v", identities)
	}
}
func TestWorkloadIdentityRejectsInvalidResponses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	os.WriteFile(path, []byte("external"), 0600)
	for _, response := range []string{`{"error":"invalid_grant"}`, `{"access_token":"token","token_type":"Bearer","expires_in":0}`, `{"access_token":"token","token_type":"Other","expires_in":900}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(response)) }))
		c := &workloadCredentials{options: WorkloadIdentityOptions{TokenFile: path, BindingID: "binding", STSBaseURL: server.URL}, client: server.Client()}
		if _, err := c.get(context.Background()); err == nil {
			t.Errorf("accepted invalid response %s", response)
		}
		server.Close()
	}
}
