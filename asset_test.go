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

func TestAssetNew(t *testing.T) {
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
	_, err := client.Assets.New(context.TODO(), limrunv1.AssetNewParams{
		Name: "name",
	})
	if err != nil {
		var apierr *limrunv1.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestAssetGetWithOptionalParams(t *testing.T) {
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
	_, err := client.Assets.Get(
		context.TODO(),
		"assetId",
		limrunv1.AssetGetParams{
			IncludeDownloadURL: limrunv1.Bool(true),
			IncludeUploadURL:   limrunv1.Bool(true),
		},
	)
	if err != nil {
		var apierr *limrunv1.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}

func TestAssetListWithOptionalParams(t *testing.T) {
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
	_, err := client.Assets.List(context.TODO(), limrunv1.AssetListParams{
		IncludeDownloadURL: limrunv1.Bool(true),
		IncludeUploadURL:   limrunv1.Bool(true),
		Md5Filter:          limrunv1.String("md5Filter"),
		NameFilter:         limrunv1.String("nameFilter"),
	})
	if err != nil {
		var apierr *limrunv1.Error
		if errors.As(err, &apierr) {
			t.Log(string(apierr.DumpRequest(true)))
		}
		t.Fatalf("err should be nil: %s", err.Error())
	}
}
