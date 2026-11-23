package limrun_test

import (
	"context"
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
