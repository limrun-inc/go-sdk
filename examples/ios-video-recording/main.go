package main

import (
	"context"
	"fmt"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
	"github.com/limrun-inc/go-sdk/websocket/ios"
)

func main() { exampleutil.Main(run) }

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
	defer exampleutil.Cleanup(func(ctx context.Context) error { return api.IosInstances.Delete(ctx, instance.Metadata.ID) })
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
	if err := exampleutil.Retry(ctx, 15*time.Second, time.Second, func(ctx context.Context) error {
		_, err := client.TapElement(ctx, ios.AccessibilitySelector{Label: "General"})
		return err
	}); err != nil {
		return err
	}
	if err := exampleutil.Sleep(ctx, time.Second); err != nil {
		return err
	}
	if err := client.OpenURL(ctx, "https://apple.com"); err != nil {
		return err
	}
	if err := exampleutil.Sleep(ctx, 10*time.Second); err != nil {
		return err
	}
	if err := client.TerminateApp(ctx, "com.apple.mobilesafari"); err != nil {
		return err
	}
	if err := exampleutil.Sleep(ctx, time.Second); err != nil {
		return err
	}
	if _, err := client.StopRecording(ctx, ios.SaveRecordingTo{LocalPath: "video.mp4"}); err != nil {
		return err
	}
	fmt.Println("Saved video.mp4")
	return nil
}
