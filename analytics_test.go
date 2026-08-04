// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package limrun_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/internal/testutil"
	"github.com/limrun-inc/go-sdk/option"
)

func TestAnalyticsGetWithOptionalParams(t *testing.T) {
	t.Skip("Mock server tests are disabled")
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
	_, err := client.Analytics.Get(context.TODO(), limrun.AnalyticsGetParams{
		From:     time.Now(),
		To:       time.Now(),
		Bucket:   limrun.AnalyticsGetParamsBucketHour,
		Labels:   limrun.String("labels"),
		Region:   limrun.String("region"),
		Timezone: limrun.String("timezone"),
	})
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestAnalyticsGetInstancesWithOptionalParams(t *testing.T) {
	t.Skip("Mock server tests are disabled")
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
	_, err := client.Analytics.GetInstances(context.TODO(), limrun.AnalyticsGetInstancesParams{
		From:     time.Now(),
		To:       time.Now(),
		Labels:   limrun.String("labels"),
		Region:   limrun.String("region"),
		Timezone: limrun.String("timezone"),
	})
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}
