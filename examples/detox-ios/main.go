package main

import (
	"context"
	"fmt"
	"os"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
)

func main() { exampleutil.Main(run) }

func run(ctx context.Context) error {
	if os.Getenv("EXPO_URL") == "" {
		return fmt.Errorf("set EXPO_URL to your running Expo app")
	}
	api := limrun.NewClient()
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.IosInstanceNewParamsMetadata{Labels: map[string]string{"name": "detox-ios-example"}},
		Spec: limrun.IosInstanceNewParamsSpec{
			InitialAssets: []limrun.IosInstanceNewParamsSpecInitialAsset{
				{Kind: "App", Source: "AssetName", AssetName: limrun.String("appstore/Expo-Go-54.0.6.tar.gz")},
			},
		},
	})
	if err != nil {
		return err
	}
	if os.Getenv("LIMRUN_KEEP_INSTANCE") != "true" {
		defer exampleutil.Cleanup(func(ctx context.Context) error { return api.IosInstances.Delete(ctx, instance.Metadata.ID) })
	}
	fmt.Printf("Open https://console.limrun.com/stream/%s\n", instance.Metadata.ID)
	return exampleutil.RunNode(ctx, "examples/detox-ios", "runner.mjs", map[string]string{
		"id":     instance.Metadata.ID,
		"apiUrl": instance.Status.APIURL,
		"token":  instance.Status.Token,
	})
}
