// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package limrun_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/internal/testutil"
	"github.com/limrun-inc/go-sdk/option"
)

func TestGradleInstanceNewWithOptionalParams(t *testing.T) {
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
	_, err := client.GradleInstances.New(context.TODO(), limrun.GradleInstanceNewParams{
		ReuseIfExists: limrun.Bool(true),
		Wait:          limrun.Bool(true),
		Metadata: limrun.GradleInstanceNewParamsMetadata{
			DisplayName: limrun.String("displayName"),
			Labels: map[string]string{
				"foo": "string",
			},
		},
		Spec: limrun.GradleInstanceNewParamsSpec{
			Clues: []limrun.GradleInstanceNewParamsSpecClue{{
				Kind:     "ClientIP",
				ClientIP: limrun.String("clientIp"),
			}},
			HardTimeout:       limrun.String("hardTimeout"),
			InactivityTimeout: limrun.String("inactivityTimeout"),
			Jurisdiction:      "us",
			Region:            limrun.String("region"),
		},
	})
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestGradleInstanceListWithOptionalParams(t *testing.T) {
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
	_, err := client.GradleInstances.List(context.TODO(), limrun.GradleInstanceListParams{
		EndingBefore:  limrun.String("endingBefore"),
		LabelSelector: limrun.String("env=prod,version=1.2"),
		Limit:         limrun.Int(50),
		StartingAfter: limrun.String("startingAfter"),
		State:         limrun.String("assigned,ready"),
	})
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestGradleInstanceDelete(t *testing.T) {
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
	err := client.GradleInstances.Delete(context.TODO(), "id")
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestGradleInstanceGet(t *testing.T) {
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
	_, err := client.GradleInstances.Get(context.TODO(), "id")
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}
