package main

import (
	"context"
	"fmt"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
	"github.com/limrun-inc/go-sdk/tunnel"
)

func main() { exampleutil.Main(run) }

func run(ctx context.Context) error {
	api := limrun.NewClient()
	instance, err := api.AndroidInstances.New(ctx, limrun.AndroidInstanceNewParams{Wait: limrun.Bool(true)})
	if err != nil {
		return err
	}
	defer exampleutil.Cleanup(func(ctx context.Context) error { return api.AndroidInstances.Delete(ctx, instance.Metadata.ID) })
	adb, err := tunnel.NewADB(instance.Status.AdbWebSocketURL, instance.Status.Token)
	if err != nil {
		return err
	}
	defer adb.Close()
	if err := adb.Start(); err != nil {
		return err
	}
	fmt.Printf("Run: adb connect %s\nTunnel closes in five minutes or on Ctrl+C.\n", adb.Addr())
	return exampleutil.Sleep(ctx, 5*time.Minute)
}
