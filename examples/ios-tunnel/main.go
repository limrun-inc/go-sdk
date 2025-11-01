package main

import (
	"log"
	"net/url"
	"time"

	"github.com/limrun-inc/go-sdk/tunnel"
)

func main() {
	init := time.Now()
	log.Printf("Instance created in %s\n", time.Since(init))
	u, err := url.Parse("ws://10.244.1.2:8833/port-forward")
	if err != nil {
		log.Fatalf("failed to parse url: %s", err)
	}
	t, err := tunnel.NewMultiplexed(u, 8100, "ola", tunnel.MultiplexedWithLocalPort(8282))
	if err != nil {
		log.Fatalf("failed to start tunnel: %s", err)
	}
	defer t.Close()
	if err := t.Start(); err != nil {
		log.Fatalf("failed to start tunnel: %s", err)
	}
	log.Printf("The port %d for the iOS simulator is available at %s", 8100, t.Addr())
	log.Printf("Will close after 5 minutes")
	time.Sleep(5 * time.Minute)
}
