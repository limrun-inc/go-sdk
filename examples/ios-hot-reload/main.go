package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/syncfolder"
	"github.com/limrun-inc/go-sdk/websocket/ios"
)

var watch = flag.Bool("watch", false, "watch the app bundle and sync changes")

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
	if flag.NArg() != 1 {
		return fmt.Errorf("usage: go run ./examples/ios-hot-reload [-watch] /path/to/App.app")
	}
	api := limrun.NewClient()
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.IosInstanceNewParamsMetadata{Labels: map[string]string{"name": "ios-hot-reload-example"}},
	})
	if err != nil {
		return err
	}
	client, err := api.IosInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	fmt.Printf("Open https://console.limrun.com/stream/%s\n", instance.Metadata.ID)
	synced, err := client.SyncApp(ctx, flag.Arg(0), ios.SyncAppOptions{Watch: watch, LaunchMode: syncfolder.LaunchRelaunchIfRunning})
	if err != nil {
		return err
	}
	if synced.StopWatching != nil {
		defer synced.StopWatching()
	}
	bundleID := synced.InstalledBundleID
	if bundleID == "" {
		return fmt.Errorf("sync did not return an installed bundle ID")
	}
	appLog, err := client.StreamAppLog(ctx, bundleID)
	if err != nil {
		return err
	}
	if err := printLogs(ctx, appLog, ""); err != nil {
		return err
	}
	syslog, err := client.StreamSyslog(ctx)
	if err != nil {
		return err
	}
	if err := printLogs(ctx, syslog, bundleID); err != nil {
		return err
	}
	tail, err := client.AppLogTail(ctx, bundleID, 10)
	if err != nil {
		return err
	}
	fmt.Println(tail)
	if *watch {
		fmt.Println("Watching; press Ctrl+C to stop syncing.")
		<-ctx.Done()
	}
	return nil
}

func printLogs(ctx context.Context, stream *ios.LogStream, contains string) error {
	defer stream.Stop()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		case lines, ok := <-stream.Lines():
			if !ok {
				return <-stream.Err()
			}
			for _, line := range lines {
				if strings.Contains(line, contains) {
					fmt.Println(line)
				}
			}
		}
	}
}
