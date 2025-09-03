package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/limrun-inc/go-sdk/api"
	"github.com/limrun-inc/go-sdk/tunnel"
)

func main() {
	organizationId := os.Getenv("ORGANIZATION_ID") // org_somevalue
	token := os.Getenv("LIM_TOKEN")                // lim_somevalue
	ctx := context.TODO()

	limrun, err := api.NewClient("https://edge.limrun.net", api.WithToken(token))
	if err != nil {
		log.Fatalf("failed to create limrun client: %s", err)
	}

	init := time.Now()
	instance, err := limrun.CreateAndroidInstance(ctx, &api.AndroidInstanceCreate{}, api.CreateAndroidInstanceParams{
		OrganizationId: organizationId,
		Wait:           api.NewOptBool(true),
	})
	if err != nil {
		log.Fatalf("failed to create an android instance: %s", err)
	}
	fmt.Printf("Instance created in %s\n", time.Since(init))

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
