package limrun_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
)

func TestSupportTicketRetry(t *testing.T) {
	for _, supplied := range []string{"", uuid.NewString()} {
		var keys []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keys = append(keys, r.Header.Get("Idempotency-Key"))
			w.Header().Set("Content-Type", "application/json")
			if len(keys) == 1 {
				w.Header().Set("Retry-After-Ms", "1")
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"id":"ticket_test"}`))
		}))
		client := limrun.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL))
		ticket, err := client.SupportTickets.Create(context.Background(), "org_test", limrun.SupportTicketCreateParams{Kind: "ticket", Subject: "Build failed", Message: "Signing failed.", IdempotencyKey: supplied})
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if ticket.ID != "ticket_test" || len(keys) != 2 || keys[0] != keys[1] || (supplied != "" && keys[0] != supplied) {
			t.Fatalf("ticket %s, idempotency keys %v", ticket.ID, keys)
		}
		if _, err := uuid.Parse(keys[0]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSupportTicketSetCC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "*/*" {
			t.Errorf("Accept = %q, want */*", got)
		}
		if r.Method != "PATCH" || r.URL.Path != "/v1/organizations/org_test/support_tickets/ticket_test" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var payload struct {
			CC []string `json:"ccEmails"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.CC == nil || len(payload.CC) != 0 {
			t.Errorf("invalid clear request: %+v, %v", payload, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := limrun.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL))
	if err := client.SupportTickets.SetCC(context.Background(), "org_test", "ticket_test", nil); err != nil {
		t.Fatal(err)
	}
}
