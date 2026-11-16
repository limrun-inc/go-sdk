package option

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// WorkloadIdentityOptions exchanges a customer-issued JWT for a short-lived Limrun token.
// TokenFile is read again on renewal, so the identity provider can rotate it atomically.
type WorkloadIdentityOptions struct {
	TokenFile string
	BindingID string
	// STSBaseURL defaults to https://sts.limrun.com. Use only a trusted token service.
	STSBaseURL string
}

// WithWorkloadIdentityFile authenticates requests without storing a Limrun API key.
func WithWorkloadIdentityFile(options WorkloadIdentityOptions) RequestOption {
	credentials := &workloadCredentials{options: options, client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("workload token exchange must not redirect")
	}}}
	return WithMiddleware(func(req *http.Request, next MiddlewareNext) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			return next(req)
		}
		token, err := credentials.get(req.Context())
		if err != nil {
			return nil, err
		}
		clone := req.Clone(req.Context())
		clone.Header.Set("Authorization", "Bearer "+token)
		return next(clone)
	})
}

type workloadCredentials struct {
	options   WorkloadIdentityOptions
	client    *http.Client
	mu        sync.Mutex
	token     string
	refreshAt time.Time
	pending   chan struct{}
}

func (c *workloadCredentials) get(ctx context.Context) (string, error) {
	for {
		c.mu.Lock()
		if time.Now().Before(c.refreshAt) {
			token := c.token
			c.mu.Unlock()
			return token, nil
		}
		if c.pending != nil {
			pending := c.pending
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-pending:
				continue
			}
		}
		c.pending = make(chan struct{})
		c.mu.Unlock()
		token, expires, err := c.exchange(ctx)
		c.mu.Lock()
		if err == nil {
			c.token = token
			lead := time.Until(expires) / 5
			if lead > time.Minute {
				lead = time.Minute
			}
			c.refreshAt = expires.Add(-lead)
		}
		close(c.pending)
		c.pending = nil
		c.mu.Unlock()
		return token, err
	}
}
func (c *workloadCredentials) exchange(ctx context.Context) (string, time.Time, error) {
	if c.options.TokenFile == "" || c.options.BindingID == "" {
		return "", time.Time{}, errors.New("workload identity requires a token file and federation binding ID")
	}
	file, err := os.Open(c.options.TokenFile)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read workload identity file: %w", err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 32769))
	if err != nil {
		return "", time.Time{}, errors.New("could not read workload identity file")
	}
	jwt := strings.TrimSpace(string(raw))
	if jwt == "" || len(raw) > 32768 {
		return "", time.Time{}, errors.New("workload identity file must contain a JWT of at most 32768 bytes")
	}
	endpoint := c.options.STSBaseURL
	if endpoint == "" {
		endpoint = "https://sts.limrun.com"
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:token-exchange"}, "subject_token_type": {"urn:ietf:params:oauth:token-type:jwt"}, "subject_token": {jwt}, "audience": {c.options.BindingID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/v1/access_tokens", strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, errors.New("invalid token service URL")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	started := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("workload token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", time.Time{}, fmt.Errorf("workload token exchange returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&result); err != nil {
		return "", time.Time{}, errors.New("invalid workload token exchange response")
	}
	if result.AccessToken == "" || result.TokenType != "Bearer" || result.ExpiresIn < 1 || result.ExpiresIn > 14400 {
		return "", time.Time{}, errors.New("invalid workload access token or expiration")
	}
	expires := started.Add(time.Duration(result.ExpiresIn) * time.Second)
	if !expires.After(time.Now()) {
		return "", time.Time{}, errors.New("exchanged token has already expired")
	}
	return result.AccessToken, expires, nil
}
