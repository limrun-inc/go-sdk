package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
)

func main() { exampleutil.Main(run) }

func run(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "limrun-asset-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	archive := filepath.Join(dir, "Expo-Go-54.0.6.tar.gz")
	if err := exampleutil.Download(ctx, "https://github.com/expo/expo-go-releases/releases/download/Expo-Go-54.0.6/Expo-Go-54.0.6.tar.gz", archive); err != nil {
		return err
	}
	api := limrun.NewClient()
	// GetOrUpload reuses an asset when its content has already been uploaded.
	asset, err := api.Assets.GetOrUpload(ctx, limrun.AssetGetOrUploadParams{Path: archive})
	if err != nil {
		return err
	}
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.IosInstanceNewParamsMetadata{Labels: map[string]string{"name": "expo-go-ios-simulator-example"}},
		Spec: limrun.IosInstanceNewParamsSpec{
			InitialAssets: []limrun.IosInstanceNewParamsSpecInitialAsset{
				{Kind: "App", Source: "AssetName", AssetName: limrun.String(asset.Name)},
			},
		},
	})
	if err != nil {
		return err
	}
	client, err := api.IosInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	fmt.Printf("Uploaded %s; Expo Go is installed.\nOpen https://console.limrun.com/stream/%s\n", asset.Name, instance.Metadata.ID)
	// Keep the instance available for interactive use, as in the TypeScript example.
	return nil
}
