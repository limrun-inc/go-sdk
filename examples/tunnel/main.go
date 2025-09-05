package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/limrun-inc/go-sdk/api"
	"github.com/limrun-inc/go-sdk/tunnel"
)

func main() {
	token := os.Getenv("LIM_TOKEN") // lim_yourtoken
	limrun := api.NewDefaultClient(token)

	init := time.Now()
	ctx := context.TODO()
	instance, err := limrun.CreateAndroidInstance(ctx, &api.AndroidInstanceCreate{}, api.CreateAndroidInstanceParams{
		Wait: api.NewOptBool(true),
	})
	if err != nil {
		log.Fatalf("failed to create an android instance: %s", err)
	}
	log.Printf("Instance created in %s\n", time.Since(init))

	t, err := tunnel.New(instance.Status.AdbWebSocketUrl.Value, instance.Status.Token)
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
