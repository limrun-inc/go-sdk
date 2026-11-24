package option

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
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

type exchangeTransport func(*http.Request) (*http.Response, error)

func (f exchangeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWorkloadIdentitySharesFailedRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path, []byte("external"), 0600); err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var calls atomic.Int32
		c := &workloadCredentials{
			options: WorkloadIdentityOptions{TokenFile: path, BindingID: "binding"},
			client: &http.Client{Transport: exchangeTransport(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				<-release
				return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			})},
		}
		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				if _, err := c.get(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
					t.Errorf("expected shared exchange failure, got %v", err)
				}
			})
		}
		synctest.Wait()
		close(release)
		wg.Wait()
		if calls.Load() != 1 {
			t.Fatalf("%d exchanges, want 1", calls.Load())
		}
		c.client.Transport = exchangeTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"recovered","token_type":"Bearer","expires_in":900}`)), Header: make(http.Header)}, nil
		})
		if token, err := c.get(context.Background()); token != "recovered" || err != nil {
			t.Fatalf("later request did not recover: %q, %v", token, err)
		}
	})
}

// subjectServer records each exchanged subject token and answers with a short session.
func subjectServer(t *testing.T, subjects *[]string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		mu.Lock()
		*subjects = append(*subjects, r.Form.Get("subject_token"))
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"access_token": "limrun-session", "token_type": "Bearer", "expires_in": 900})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestWorkloadIdentitySources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path, []byte("from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := func(context.Context) (string, error) { return " from-source ", nil }
	for _, tc := range []struct {
		name    string
		options WorkloadIdentityOptions
		want    string
	}{
		{"token", WorkloadIdentityOptions{Token: "from-token"}, "from-token"},
		{"token source", WorkloadIdentityOptions{TokenSource: source}, "from-source"},
		{"token beats file", WorkloadIdentityOptions{Token: "from-token", TokenFile: path}, "from-token"},
		{"token source beats token", WorkloadIdentityOptions{TokenSource: source, Token: "from-token"}, "from-source"},
		{"file", WorkloadIdentityOptions{TokenFile: path}, "from-file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var subjects []string
			server := subjectServer(t, &subjects)
			tc.options.BindingID, tc.options.STSBaseURL = "binding_customer", server.URL
			c := &workloadCredentials{options: tc.options, client: server.Client()}
			if _, err := c.get(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(subjects) != 1 || subjects[0] != tc.want {
				t.Fatalf("exchanged %q, want %q", subjects, tc.want)
			}
		})
	}
}

func TestWorkloadTokenSourceRunsForEveryExchange(t *testing.T) {
	var subjects []string
	server := subjectServer(t, &subjects)
	issued := 0
	c := &workloadCredentials{options: WorkloadIdentityOptions{
		TokenSource: func(context.Context) (string, error) { issued++; return "jwt-" + strconv.Itoa(issued), nil },
		BindingID:   "binding_customer", STSBaseURL: server.URL,
	}, client: server.Client()}
	for range 2 {
		if _, err := c.get(context.Background()); err != nil {
			t.Fatal(err)
		}
		c.mu.Lock()
		c.refreshAt = time.Time{}
		c.mu.Unlock()
	}
	if len(subjects) != 2 || subjects[0] != "jwt-1" || subjects[1] != "jwt-2" {
		t.Fatalf("exchanged %q, want a fresh JWT each time", subjects)
	}
}

func TestWorkloadIdentitySourceErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options WorkloadIdentityOptions
		want    string
	}{
		{"no source", WorkloadIdentityOptions{}, "requires a token, token source, or token file"},
		{"source failure", WorkloadIdentityOptions{TokenSource: func(context.Context) (string, error) {
			return "", io.ErrUnexpectedEOF
		}}, "get workload identity token: unexpected EOF"},
		{"blank token", WorkloadIdentityOptions{TokenSource: func(context.Context) (string, error) { return "  ", nil }}, "must be a JWT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.options.BindingID = "binding_customer"
			c := &workloadCredentials{options: tc.options, client: &http.Client{Transport: exchangeTransport(func(*http.Request) (*http.Response, error) {
				t.Error("exchange must not run")
				return nil, io.EOF
			})}}
			if _, err := c.get(context.Background()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}
