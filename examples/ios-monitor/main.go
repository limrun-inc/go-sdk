package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/websocket/ios"
)

func main() {
	instanceID := flag.String("instance", "", "ready iOS instance ID")
	bundleID := flag.String("bundle", "", "installed app bundle ID")
	flag.Parse()
	if *instanceID == "" || *bundleID == "" {
		flag.Usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := monitor(ctx, *instanceID, *bundleID); err != nil {
		log.Fatal(err)
	}
}

func monitor(ctx context.Context, instanceID, bundleID string) error {
	api := limrun.NewClient()
	instance, err := api.IosInstances.Get(ctx, instanceID)
	if err != nil {
		return err
	}
	client, err := api.IosInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()

	watch, err := client.MonitorApp(ctx, bundleID, ios.AppMonitorCallbacks{
		OnExit: func(lines []string, info ios.AppExitInfo) {
			log.Printf("%s exited: %s", info.BundleID, info.Reason)
			if info.Crash != nil {
				log.Printf("crash: %s\n%s", info.Crash.LongMsg, info.Crash.StackTrace)
			}
			for _, line := range lines {
				log.Print(line)
			}
		},
		OnHang: func(info ios.AppHangInfo) {
			log.Printf("%s hang %d: duration=%gms ended=%t", info.BundleID, info.HangID, info.DurationMs, info.Ended)
		},
	})
	if err != nil {
		return err
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = watch.Stop(stopCtx)
	}()

	syslog, err := client.StreamSyslog(ctx)
	if err != nil {
		return err
	}
	defer syslog.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case lines, ok := <-syslog.Lines():
			if !ok {
				return <-syslog.Err()
			}
			for _, line := range lines {
				fmt.Println(line)
			}
		}
	}
}
