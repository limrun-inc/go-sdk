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
		Metadata:      limrun.AndroidInstanceNewParamsMetadata{Labels: map[string]string{"name": "websocket-android-example"}},
	})
	if err != nil {
		return err
	}
	client, err := api.AndroidInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	shot, err := client.Screenshot(ctx)
	if err != nil {
		return err
	}
	if err := exampleutil.SaveBase64("android-screenshot.png", shot.DataURI); err != nil {
		return err
	}
	tree, err := client.GetElementTree(ctx, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Element tree: %d XML bytes, %d nodes\n", len(tree.XML), len(tree.Nodes))
	matches, err := client.FindElement(ctx, android.Selector{Text: exampleutil.Ptr("Chrome")}, 5)
	if err != nil {
		return err
	}
	fmt.Printf("Chrome matches: %d\n", matches.Count)
	if _, err := client.OpenURL(ctx, "https://example.com"); err != nil {
		return err
	}
	if err := exampleutil.Sleep(ctx, 2*time.Second); err != nil {
		return err
	}
	if _, err := client.PressKey(ctx, "BACK"); err != nil {
		return err
	}
	if err := exampleutil.Sleep(ctx, time.Second); err != nil {
		return err
	}
	for _, direction := range []android.ScrollDirection{android.ScrollDown, android.ScrollUp} {
		if _, err := client.ScrollScreen(ctx, direction, 10); err != nil {
			return err
		}
		if err := exampleutil.Sleep(ctx, time.Second); err != nil {
			return err
		}
	}
	if _, err := client.Tap(ctx, android.ElementTarget{Selector: &android.Selector{Clickable: exampleutil.Ptr(true)}}); err != nil {
		fmt.Printf("No clickable element: %v\n", err)
	}
	if _, err := client.SetText(ctx, &android.ElementTarget{
		Selector: &android.Selector{ClassName: "android.widget.EditText", Enabled: exampleutil.Ptr(true)},
	}, "Hello from Go SDK!"); err != nil {
		fmt.Printf("No editable element: %v\n", err)
	}
	final, err := client.Screenshot(ctx)
	if err != nil {
		return err
	}
	if err := exampleutil.SaveBase64("android-screenshot-final.png", final.DataURI); err != nil {
		return err
	}
	fmt.Printf("Screenshots saved. Instance %s remains available.\n", instance.Metadata.ID)
	return nil
}
