package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
	defer cleanup(func(ctx context.Context) error { return api.AndroidInstances.Delete(ctx, instance.Metadata.ID) })
	endpoint := instance.Status.Sandbox.PlaywrightAndroid.URL
	if endpoint == "" {
		return fmt.Errorf("instance has no Playwright Android sandbox URL")
	}
	return runNode(ctx, "examples/playwright", "runner.mjs", map[string]string{"url": endpoint, "token": instance.Status.Token})
}

// cleanup still runs when Ctrl+C has canceled the example's main context.
func cleanup(fn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := fn(ctx); err != nil {
		log.Printf("cleanup: %v", err)
	}
}

// runNode sends instance credentials through stdin, not command-line arguments.
func runNode(ctx context.Context, directory, script string, input any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Dir = directory
	cmd.Stdin = strings.NewReader(string(data))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
