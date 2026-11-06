package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
	"github.com/limrun-inc/go-sdk/tunnel"
)

func main() { exampleutil.Main(run) }

func run(ctx context.Context) error {
	api := limrun.NewClient()
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{Wait: limrun.Bool(true)})
	if err != nil {
		return err
	}
	defer exampleutil.Cleanup(func(ctx context.Context) error { return api.IosInstances.Delete(ctx, instance.Metadata.ID) })
	client, err := api.IosInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.OpenURL(ctx, "https://example.com"); err != nil {
		return err
	}
	var socket string
	if err := exampleutil.Retry(ctx, 30*time.Second, 250*time.Millisecond, func(ctx context.Context) error {
		files, err := client.Lsof(ctx)
		if err != nil {
			return err
		}
		for _, file := range files {
			if strings.HasSuffix(file.Path, "com.apple.webinspectord_sim.socket") {
				socket = file.Path
				return nil
			}
		}
		return fmt.Errorf("Safari Web Inspector socket is not available yet")
	}); err != nil {
		return err
	}
	remote, err := url.Parse(strings.TrimRight(instance.Status.APIURL, "/") + "/port-forward")
	if err != nil {
		return err
	}
	if remote.Scheme == "https" {
		remote.Scheme = "wss"
	} else {
		remote.Scheme = "ws"
	}
	query := remote.Query()
	query.Set("socketPath", socket)
	remote.RawQuery = query.Encode()
	forward, err := tunnel.StartTCP(ctx, remote.String(), instance.Status.Token, "127.0.0.1", 27753, tunnel.TCPOptions{Mode: "multiplexed"})
	if err != nil {
		return err
	}
	defer forward.Close()
	disconnected := make(chan struct{}, 1)
	unsubscribe := forward.OnConnectionStateChange(func(state tunnel.ConnectionState) {
		if state == tunnel.Disconnected {
			select {
			case disconnected <- struct{}{}:
			default:
			}
		}
	})
	defer unsubscribe()
	if forward.GetConnectionState() == tunnel.Disconnected {
		return fmt.Errorf("Web Inspector tunnel disconnected")
	}
	fmt.Println("Connect a Web Inspector protocol client to 127.0.0.1:27753. Press Ctrl+C to stop.")
	select {
	case <-ctx.Done():
		return nil
	case <-disconnected:
		return fmt.Errorf("Web Inspector tunnel disconnected")
	}
}
