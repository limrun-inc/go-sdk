package main

import (
	"context"
	"fmt"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
	"github.com/limrun-inc/go-sdk/websocket/android"
)

func main() { exampleutil.Main(run) }

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
	defer exampleutil.Cleanup(func(ctx context.Context) error { return api.AndroidInstances.Delete(ctx, instance.Metadata.ID) })
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
	if err := exampleutil.Sleep(ctx, 5*time.Second); err != nil {
		return err
	}
	if _, err := client.PressKey(ctx, "HOME"); err != nil {
		return err
	}
	if err := exampleutil.Sleep(ctx, time.Second); err != nil {
		return err
	}
	if _, err := client.OpenURL(ctx, "https://example.com"); err != nil {
		return err
	}
	if err := exampleutil.Sleep(ctx, 5*time.Second); err != nil {
		return err
	}
	if _, err := client.StopRecording(ctx, &android.RecordingDestination{LocalPath: "android-video.mp4"}); err != nil {
		return err
	}
	fmt.Println("Saved android-video.mp4")
	return nil
}
