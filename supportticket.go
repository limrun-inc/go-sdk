package limrun

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/option"
)

type SupportTicketService struct {
	Options []option.RequestOption
}

func NewSupportTicketService(opts ...option.RequestOption) SupportTicketService {
	return SupportTicketService{Options: opts}
}

type SupportTicket struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	Subject        string    `json:"subject"`
	CreatedAt      time.Time `json:"createdAt"`
}

type SupportTicketCreateParams struct {
	// Kind is "ticket" or "feedback".
	Kind     string   `json:"kind"`
	Subject  string   `json:"subject"`
	Message  string   `json:"message"`
	CCEmails []string `json:"ccEmails,omitempty"`
	// Reuse this UUID across deliveries of the same event. Create generates one when omitted.
	IdempotencyKey string `json:"-"`
}

func (r *SupportTicketService) Create(ctx context.Context, organizationID string, params SupportTicketCreateParams, opts ...option.RequestOption) (*SupportTicket, error) {
	key := params.IdempotencyKey
	if key == "" {
		key = uuid.NewString()
	}
	options := slices.Concat(r.Options, []option.RequestOption{option.WithHeader("Idempotency-Key", key)}, opts)
	path := "v1/organizations/" + url.PathEscape(organizationID) + "/support_tickets"
	var ticket *SupportTicket
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, params, &ticket, options...)
	return ticket, err
}

// SetCC replaces the recipients of future support notifications. An empty list clears them.
func (r *SupportTicketService) SetCC(ctx context.Context, organizationID, ticketID string, ccEmails []string, opts ...option.RequestOption) error {
	path := "v1/organizations/" + url.PathEscape(organizationID) + "/support_tickets/" + url.PathEscape(ticketID)
	if ccEmails == nil {
		ccEmails = []string{}
	}
	options := slices.Concat(r.Options, []option.RequestOption{option.WithHeader("Accept", "*/*")}, opts)
	return requestconfig.ExecuteNewRequest(ctx, http.MethodPatch, path, map[string][]string{"ccEmails": ccEmails}, nil, options...)
}
