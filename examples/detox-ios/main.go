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
		defer cleanup(func(ctx context.Context) error { return api.IosInstances.Delete(ctx, instance.Metadata.ID) })
	}
	fmt.Printf("Open https://console.limrun.com/stream/%s\n", instance.Metadata.ID)
	return runNode(ctx, "examples/detox-ios", "runner.mjs", map[string]string{
		"id":     instance.Metadata.ID,
		"apiUrl": instance.Status.APIURL,
		"token":  instance.Status.Token,
	})
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
