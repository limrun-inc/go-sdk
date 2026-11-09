package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/websocket/ios"
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
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.IosInstanceNewParamsMetadata{Labels: map[string]string{"name": "ios-video-recording-example"}},
	})
	if err != nil {
		return err
	}
	defer cleanup(func(ctx context.Context) error { return api.IosInstances.Delete(ctx, instance.Metadata.ID) })
	client, err := api.IosInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.StartRecording(ctx, nil); err != nil {
		return err
	}
	if err := client.LaunchApp(ctx, "com.apple.Preferences"); err != nil {
		return err
	}
	if err := retry(ctx, 15*time.Second, time.Second, func(ctx context.Context) error {
		_, err := client.TapElement(ctx, ios.AccessibilitySelector{Label: "General"})
		return err
	}); err != nil {
		return err
	}
	if err := sleep(ctx, time.Second); err != nil {
		return err
	}
	if err := client.OpenURL(ctx, "https://apple.com"); err != nil {
		return err
	}
	if err := sleep(ctx, 10*time.Second); err != nil {
		return err
	}
	if err := client.TerminateApp(ctx, "com.apple.mobilesafari"); err != nil {
		return err
	}
	if err := sleep(ctx, time.Second); err != nil {
		return err
	}
	if _, err := client.StopRecording(ctx, ios.SaveRecordingTo{LocalPath: "video.mp4"}); err != nil {
		return err
	}
	fmt.Println("Saved video.mp4")
	return nil
}

// cleanup still runs when Ctrl+C has canceled the example's main context.
func cleanup(fn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := fn(ctx); err != nil {
		log.Printf("cleanup: %v", err)
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retry(ctx context.Context, timeout, delay time.Duration, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("retry stopped: %w (last attempt: %v)", ctx.Err(), err)
		case <-timer.C:
		}
	}
}
