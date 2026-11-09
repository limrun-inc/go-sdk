package limrun_test

import (
	"context"
	"errors"
	"os"
	"testing"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/internal/testutil"
	"github.com/limrun-inc/go-sdk/option"
)

func TestAccessTokenNewWithOptionalParams(t *testing.T) {
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
	_, err := client.AccessTokens.New(context.TODO(), limrun.AccessTokenNewParams{
		Permissions: []string{"applerelay:*:connect"},
		TtlSeconds:  limrun.Int(1),
	})
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestAccessTokenRevoke(t *testing.T) {
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
	_, err := client.AccessTokens.Revoke(context.TODO(), "tokenId")
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}
