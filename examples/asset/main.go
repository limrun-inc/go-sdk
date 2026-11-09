package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	limrun "github.com/limrun-inc/go-sdk"
)

func main() {
	if os.Getenv("LIM_API_KEY") == "" {
		log.Fatal("set LIM_API_KEY before running this example")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx)
	stop()
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "limrun-asset-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	archive := filepath.Join(dir, "Expo-Go-54.0.6.tar.gz")
	if err := download(ctx, "https://github.com/expo/expo-go-releases/releases/download/Expo-Go-54.0.6/Expo-Go-54.0.6.tar.gz", archive); err != nil {
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

func download(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", res.Status)
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, res.Body)
	return errors.Join(copyErr, file.Close())
}
