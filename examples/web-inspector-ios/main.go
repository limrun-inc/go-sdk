package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/tunnel"
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
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{Wait: limrun.Bool(true)})
	if err != nil {
		return err
	}
	defer cleanup(func(ctx context.Context) error { return api.IosInstances.Delete(ctx, instance.Metadata.ID) })
	client, err := api.IosInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.OpenURL(ctx, "https://example.com"); err != nil {
		return err
	}
	var socket string
	if err := retry(ctx, 30*time.Second, 250*time.Millisecond, func(ctx context.Context) error {
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
		return fmt.Errorf("the Safari Web Inspector socket is not available yet")
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
		return fmt.Errorf("web inspector tunnel disconnected")
	}
	fmt.Println("Connect a Web Inspector protocol client to 127.0.0.1:27753. Press Ctrl+C to stop.")
	select {
	case <-ctx.Done():
		return nil
	case <-disconnected:
		return fmt.Errorf("web inspector tunnel disconnected")
	}
}

// cleanup still runs when Ctrl+C has canceled the example's main context.
func cleanup(fn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := fn(ctx); err != nil {
		log.Printf("cleanup: %v", err)
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
