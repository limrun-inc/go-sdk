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
	"github.com/limrun-inc/go-sdk/websocket/android"
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
		Metadata:      limrun.AndroidInstanceNewParamsMetadata{Labels: map[string]string{"name": "android-video-recording-example"}},
	})
	if err != nil {
		return err
	}
	defer cleanup(func(ctx context.Context) error { return api.AndroidInstances.Delete(ctx, instance.Metadata.ID) })
	client, err := api.AndroidInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.StartRecording(ctx, nil); err != nil {
		return err
	}
	if _, err := client.OpenURL(ctx, "https://android.com"); err != nil {
		return err
	}
	if err := sleep(ctx, 5*time.Second); err != nil {
		return err
	}
	if _, err := client.PressKey(ctx, "HOME"); err != nil {
		return err
	}
	if err := sleep(ctx, time.Second); err != nil {
		return err
	}
	if _, err := client.OpenURL(ctx, "https://example.com"); err != nil {
		return err
	}
	if err := sleep(ctx, 5*time.Second); err != nil {
		return err
	}
	if _, err := client.StopRecording(ctx, &android.RecordingDestination{LocalPath: "android-video.mp4"}); err != nil {
		return err
	}
	fmt.Println("Saved android-video.mp4")
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
