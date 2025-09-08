// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package limrunv1_test

import (
	"context"
	"os"
	"testing"

	"github.com/stainless-sdks/limrun-v1-go"
	"github.com/stainless-sdks/limrun-v1-go/internal/testutil"
	"github.com/stainless-sdks/limrun-v1-go/option"
)

func TestUsage(t *testing.T) {
	baseURL := "http://localhost:4010"
	if envURL, ok := os.LookupEnv("TEST_API_BASE_URL"); ok {
		baseURL = envURL
	}
	if !testutil.CheckTestServer(t, baseURL) {
		return
	}
	client := limrunv1.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("My API Key"),
	)
	androidInstances, err := client.AndroidInstances.List(context.TODO(), limrunv1.AndroidInstanceListParams{})
	if err != nil {
		t.Fatalf("err should be nil: %s", err.Error())
	}
	t.Logf("%+v\n", androidInstances)
}
