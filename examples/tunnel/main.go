package main

import (
	"context"
	"fmt"
	"log"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
	"github.com/limrun-inc/go-sdk/tunnel"
	"github.com/limrun-inc/go-sdk/websocket/android"
)

func main() { exampleutil.Main(run) }

func run(ctx context.Context) error {
	api := limrun.NewClient()
	instance, err := api.AndroidInstances.New(ctx, limrun.AndroidInstanceNewParams{Wait: limrun.Bool(true)})
	if err != nil {
		return err
	}
	defer exampleutil.Cleanup(func(ctx context.Context) error { return api.AndroidInstances.Delete(ctx, instance.Metadata.ID) })
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
	return exampleutil.Sleep(ctx, 30*time.Second)
}
