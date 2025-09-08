package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/tunnel"
)

func main() {
	token := os.Getenv("LIM_TOKEN") // lim_yourtoken
	lim := limrun.NewClient(option.WithAPIKey(token))

	init := time.Now()
	ctx := context.TODO()
	instance, err := lim.AndroidInstances.New(ctx, limrun.AndroidInstanceNewParams{})
	if err != nil {
		log.Fatalf("failed to create an android instance: %s", err)
	}
	log.Printf("Instance created in %s\n", time.Since(init))

	t, err := tunnel.New(instance.Status.AdbWebSocketURL, instance.Status.Token)
	if err != nil {
		log.Fatalf("failed to start tunnel: %s", err)
	}
	defer t.Close()
	if err := t.Start(); err != nil {
		log.Fatalf("failed to start tunnel: %s", err)
	}
	log.Printf("Connected to adb at %s", t.Addr())
	log.Printf("Will close after 5 minutes")
	time.Sleep(5 * time.Minute)
}
