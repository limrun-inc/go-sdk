// File generated from our OpenAPI spec by Stainless. See CONTRIBUTING.md for details.

package limrunv1_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stainless-sdks/limrun-v1-go"
	"github.com/stainless-sdks/limrun-v1-go/internal/testutil"
	"github.com/stainless-sdks/limrun-v1-go/option"
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
	client := limrunv1.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("My API Key"),
	)
	_, err := client.AndroidInstances.New(context.TODO(), limrunv1.AndroidInstanceNewParams{
		Wait: limrunv1.Bool(true),
		Metadata: limrunv1.AndroidInstanceNewParamsMetadata{
			DisplayName: limrunv1.String("displayName"),
			Labels: map[string]string{
				"foo": "string",
			},
		},
		Spec: limrunv1.AndroidInstanceNewParamsSpec{
			Clues: []limrunv1.AndroidInstanceNewParamsSpecClue{{
				Kind:     "ClientIP",
				ClientIP: limrunv1.String("clientIp"),
			}},
			HardTimeout:       limrunv1.String("hardTimeout"),
			InactivityTimeout: limrunv1.String("inactivityTimeout"),
			InitialAssets: []limrunv1.AndroidInstanceNewParamsSpecInitialAsset{{
				Kind:      "App",
				Source:    "URL",
				AssetName: limrunv1.String("assetName"),
				URL:       limrunv1.String("url"),
			}},
			Region: limrunv1.String("region"),
		},
	})
	if err != nil {
		var apierr *limrunv1.Error
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
	client := limrunv1.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("My API Key"),
	)
	_, err := client.AndroidInstances.Get(context.TODO(), "id")
	if err != nil {
		var apierr *limrunv1.Error
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
	client := limrunv1.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("My API Key"),
	)
	_, err := client.AndroidInstances.List(context.TODO(), limrunv1.AndroidInstanceListParams{
		LabelSelector: limrunv1.String("env=prod,version=1.2"),
		Region:        limrunv1.String("region"),
		State:         map[string]interface{}{},
	})
	if err != nil {
		var apierr *limrunv1.Error
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
	client := limrunv1.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("My API Key"),
	)
	err := client.AndroidInstances.Delete(context.TODO(), "id")
	if err != nil {
		var apierr *limrunv1.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}
