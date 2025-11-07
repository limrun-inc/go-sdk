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

func TestAndroidInstanceNewWithOptionalParams(t *testing.T) {
	t.Skip("Prism tests are disabled")
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
	_, err := client.AndroidInstances.New(context.TODO(), limrun.AndroidInstanceNewParams{
		Wait: limrun.Bool(true),
		Metadata: limrun.AndroidInstanceNewParamsMetadata{
			DisplayName: limrun.String("displayName"),
			Labels: map[string]string{
				"foo": "string",
			},
		},
		Spec: limrun.AndroidInstanceNewParamsSpec{
			Clues: []limrun.AndroidInstanceNewParamsSpecClue{{
				Kind:      "ClientIP",
				ClientIP:  limrun.String("clientIp"),
				OsVersion: limrun.String("osVersion"),
			}},
			HardTimeout:       limrun.String("hardTimeout"),
			InactivityTimeout: limrun.String("inactivityTimeout"),
			InitialAssets: []limrun.AndroidInstanceNewParamsSpecInitialAsset{{
				Kind:       "App",
				Source:     "URL",
				AssetName:  limrun.String("assetName"),
				AssetNames: []string{"string"},
				URL:        limrun.String("url"),
				URLs:       []string{"string"},
			}},
			Region: limrun.String("region"),
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

func TestAndroidInstanceListWithOptionalParams(t *testing.T) {
	t.Skip("Prism tests are disabled")
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
	_, err := client.AndroidInstances.List(context.TODO(), limrun.AndroidInstanceListParams{
		EndingBefore:  limrun.String("endingBefore"),
		LabelSelector: limrun.String("env=prod,version=1.2"),
		Limit:         limrun.Int(50),
		Region:        limrun.String("region"),
		StartingAfter: limrun.String("startingAfter"),
		State:         limrun.AndroidInstanceListParamsStateUnknown,
	})
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestAndroidInstanceDelete(t *testing.T) {
	t.Skip("Prism tests are disabled")
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
	err := client.AndroidInstances.Delete(context.TODO(), "id")
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestAndroidInstanceGet(t *testing.T) {
	t.Skip("Prism tests are disabled")
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
	_, err := client.AndroidInstances.Get(context.TODO(), "id")
	if err != nil {
		var apierr *limrun.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}
