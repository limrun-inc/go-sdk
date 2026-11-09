package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
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
	if err := saveBase64("android-screenshot.png", shot.DataURI); err != nil {
		return err
	}
	tree, err := client.GetElementTree(ctx, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Element tree: %d XML bytes, %d nodes\n", len(tree.XML), len(tree.Nodes))
	matches, err := client.FindElement(ctx, android.Selector{Text: ptr("Chrome")}, 5)
	if err != nil {
		return err
	}
	fmt.Printf("Chrome matches: %d\n", matches.Count)
	if _, err := client.OpenURL(ctx, "https://example.com"); err != nil {
		return err
	}
	if err := sleep(ctx, 2*time.Second); err != nil {
		return err
	}
	if _, err := client.PressKey(ctx, "BACK"); err != nil {
		return err
	}
	if err := sleep(ctx, time.Second); err != nil {
		return err
	}
	for _, direction := range []android.ScrollDirection{android.ScrollDown, android.ScrollUp} {
		if _, err := client.ScrollScreen(ctx, direction, 10); err != nil {
			return err
		}
		if err := sleep(ctx, time.Second); err != nil {
			return err
		}
	}
	if _, err := client.Tap(ctx, android.ElementTarget{Selector: &android.Selector{Clickable: ptr(true)}}); err != nil {
		fmt.Printf("No clickable element: %v\n", err)
	}
	if _, err := client.SetText(ctx, &android.ElementTarget{
		Selector: &android.Selector{ClassName: "android.widget.EditText", Enabled: ptr(true)},
	}, "Hello from Go SDK!"); err != nil {
		fmt.Printf("No editable element: %v\n", err)
	}
	final, err := client.Screenshot(ctx)
	if err != nil {
		return err
	}
	if err := saveBase64("android-screenshot-final.png", final.DataURI); err != nil {
		return err
	}
	fmt.Printf("Screenshots saved. Instance %s remains available.\n", instance.Metadata.ID)
	return nil
}

func ptr[T any](v T) *T { return &v }

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

func saveBase64(path, data string) error {
	if strings.HasPrefix(data, "data:") {
		_, data, _ = strings.Cut(data, ",")
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, decoded, 0600)
}
