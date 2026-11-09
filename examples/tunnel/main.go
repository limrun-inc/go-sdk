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
	"github.com/limrun-inc/go-sdk/tunnel"
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
	instance, err := api.AndroidInstances.New(ctx, limrun.AndroidInstanceNewParams{Wait: limrun.Bool(true)})
	if err != nil {
		return err
	}
	defer cleanup(func(ctx context.Context) error { return api.AndroidInstances.Delete(ctx, instance.Metadata.ID) })
	client, err := api.AndroidInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	har, err := tunnel.NewHARAssembler()
	if err != nil {
		return err
	}
	destination, err := client.StartTunnel(ctx, android.TunnelOptions{
		Selectors:  []string{"*.example.test"},
		Inspection: &tunnel.InspectionConfig{Enabled: true, CaptureBodies: true, Persist: true, TTLSeconds: 72 * 60 * 60},
		OnInspectionEvent: func(event tunnel.InspectionEvent) {
			entry, err := har.Add(event)
			if err != nil {
				log.Printf("HAR assembly: %v", err)
				return
			}
			if entry != nil {
				request, _ := entry["request"].(map[string]any)
				response, _ := entry["response"].(map[string]any)
				fmt.Printf("%v %v -> %v\n", request["method"], request["url"], response["status"])
			}
		},
		OnInspectionError: func(err error) { log.Printf("inspection: %v", err) },
	})
	if err != nil {
		return err
	}
	defer destination.Close()
	fmt.Printf("Tunnel %s active for 30 seconds. Send app traffic to a *.example.test destination that this computer can reach.\n", destination.TunnelID)
	return sleep(ctx, 30*time.Second)
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
