package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/tunnel"
)

const (
	remotePort = 8100
	localPort  = 8100
)

func main() {
	token := os.Getenv("LIM_API_KEY") // lim_yourtoken
	lim := limrun.NewClient(option.WithAPIKey(token))
	ctx := context.TODO()
	init := time.Now()
	instance, err := lim.IosInstances.New(ctx, limrun.IosInstanceNewParams{})
	if err != nil {
		log.Fatalf("failed to create an ios instance: %s", err)
	}
	log.Printf("Instance created in %s\n", time.Since(init))
	u, err := url.Parse(instance.Status.PortForwardWebSocketURL)
	if err != nil {
		log.Fatalf("failed to parse url: %s", err)
	}
	t, err := tunnel.NewMultiplexed(u, 8100, instance.Status.Token, tunnel.MultiplexedWithLocalPort(localPort))
	if err != nil {
		log.Fatalf("failed to start tunnel: %s", err)
	}
	defer t.Close()
	if err := t.Start(); err != nil {
		log.Fatalf("failed to start tunnel: %s", err)
	}
	log.Printf("The port %d for the iOS simulator is available at %s", remotePort, t.Addr())
	log.Printf("Will close after 5 minutes")
	time.Sleep(5 * time.Minute)
}
