package main

import (
	"context"
	"fmt"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
)

func main() { exampleutil.Main(run) }

func run(ctx context.Context) error {
	api := limrun.NewClient()
	instance, err := api.AndroidInstances.New(ctx, limrun.AndroidInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.AndroidInstanceNewParamsMetadata{Labels: map[string]string{"name": "playwright-example"}},
		Spec: limrun.AndroidInstanceNewParamsSpec{
			InitialAssets: []limrun.AndroidInstanceNewParamsSpecInitialAsset{
				{
					Kind: "Configuration",
					Configuration: limrun.AndroidInstanceNewParamsSpecInitialAssetConfiguration{
						Kind:       "ChromeFlag",
						ChromeFlag: "enable-command-line-on-non-rooted-devices@1",
					},
				},
			},
			Sandbox: limrun.AndroidInstanceNewParamsSpecSandbox{
				PlaywrightAndroid: limrun.AndroidInstanceNewParamsSpecSandboxPlaywrightAndroid{
					Enabled: limrun.Bool(true),
					Version: "1.60.0-lim.1",
				},
			},
			Clues: []limrun.AndroidInstanceNewParamsSpecClue{{Kind: "OSVersion", OsVersion: limrun.String("15")}},
		},
	})
	if err != nil {
		return err
	}
	defer exampleutil.Cleanup(func(ctx context.Context) error { return api.AndroidInstances.Delete(ctx, instance.Metadata.ID) })
	endpoint := instance.Status.Sandbox.PlaywrightAndroid.URL
	if endpoint == "" {
		return fmt.Errorf("instance has no Playwright Android sandbox URL")
	}
	return exampleutil.RunNode(ctx, "examples/playwright", "runner.mjs", map[string]string{"url": endpoint, "token": instance.Status.Token})
}
