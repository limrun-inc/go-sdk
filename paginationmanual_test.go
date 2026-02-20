// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package limrun_test

import (
	"context"
	"os"
	"testing"

	"github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/internal/testutil"
	"github.com/limrun-inc/go-sdk/option"
)

func TestManualPagination(t *testing.T) {
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
	page, err := client.AndroidInstances.List(context.TODO(), limrun.AndroidInstanceListParams{})
	if err != nil {
		t.Fatalf("err should be nil: %s", err.Error())
	}
	for _, androidInstance := range page.Items {
		t.Logf("%+v\n", androidInstance.Metadata)
	}
	// The mock server isn't going to give us real pagination
	page, err = page.GetNextPage()
	if err != nil {
		t.Fatalf("err should be nil: %s", err.Error())
	}
	if page != nil {
		for _, androidInstance := range page.Items {
			t.Logf("%+v\n", androidInstance.Metadata)
		}
	}
}
