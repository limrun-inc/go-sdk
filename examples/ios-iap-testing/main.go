package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/build"
	"github.com/limrun-inc/go-sdk/websocket/ios"
)

var bundleID = flag.String("bundle-id", "", "app bundle ID")
var storekit = flag.String("storekit", "", "local .storekit configuration; otherwise discover it")
var timeout = flag.Int("timeout", 120, "StoreKit discovery timeout in seconds")

func main() {
	flag.Parse()
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
	if flag.NArg() != 1 || *bundleID == "" {
		return fmt.Errorf("usage: go run ./examples/ios-iap-testing -bundle-id com.example.app [-storekit file.storekit] /path/to/project")
	}
	api := limrun.NewClient()
	sandbox, err := api.XcodeInstances.New(ctx, limrun.XcodeInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.XcodeInstanceNewParamsMetadata{Labels: map[string]string{"name": "ios-iap-testing-example"}},
	})
	if err != nil {
		return err
	}
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.IosInstanceNewParamsMetadata{Labels: map[string]string{"name": "ios-iap-testing-example"}},
	})
	if err != nil {
		return err
	}
	xcode, err := api.XcodeInstances.CreateClient(*sandbox, build.WithExecutionOptions(build.ExecOptions{Command: os.Stdout, Stdout: os.Stdout, Stderr: os.Stderr}))
	if err != nil {
		return err
	}
	if _, err := xcode.AttachSimulator(ctx, instance.Status.APIURL, instance.Status.Token); err != nil {
		return err
	}
	if _, err := xcode.Sync(ctx, flag.Arg(0), &build.SyncOptions{Watch: ptr(false)}); err != nil {
		return err
	}
	process, err := xcode.Xcodebuild(ctx, nil, nil)
	if err != nil {
		return err
	}
	result, err := process.Wait(ctx)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("xcodebuild exited with code %d", result.ExitCode)
	}
	client, err := api.IosInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	fmt.Printf("Open https://console.limrun.com/stream/%s and open the app's paywall.\n", instance.Metadata.ID)
	if *storekit != "" {
		data, err := os.ReadFile(*storekit)
		if err != nil {
			return err
		}
		if err := client.SetStoreKitConfig(ctx, *bundleID, data); err != nil {
			return err
		}
		fmt.Println("StoreKit configuration registered.")
	} else {
		result, err := client.DiscoverStoreKitConfig(ctx, *bundleID, &ios.StoreKitDiscoverOptions{TimeoutSeconds: *timeout})
		if err != nil {
			return err
		}
		fmt.Printf("Discovered %d items: %d products, %d subscriptions, %d subscription groups\n", result.ItemsFound, result.ProductsCount, result.SubscriptionsCount, result.SubscriptionGroupsCount)
	}
	// Keep both instances for interactive purchase testing.
	return nil
}

func ptr[T any](v T) *T { return &v }
