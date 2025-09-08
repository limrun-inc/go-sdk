package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Need to provide a path to a file to run")
	}
	apkPath := os.Args[1]
	token := os.Getenv("LIM_TOKEN") // lim_yourtoken
	lim := limrun.NewClient(option.WithAPIKey(token))

	ctx := context.TODO()
	initUpl := time.Now()
	asset, err := lim.Assets.GetOrUpload(ctx, limrun.AssetGetOrUploadParams{
		Path: apkPath,
	})
	if err != nil {
		log.Fatalf("failed to upload asset to limrun: %s", err)
	}
	log.Printf("Uploaded %s in %s", apkPath, time.Since(initUpl))
	spec := limrun.AndroidInstanceNewParamsSpec{
		InitialAssets: []limrun.AndroidInstanceNewParamsSpecInitialAsset{
			{
				Kind:      "App",
				Source:    "AssetName",
				AssetName: param.NewOpt(asset.Name),
			},
		},
	}
	init := time.Now()
	instance, err := lim.AndroidInstances.New(ctx, limrun.AndroidInstanceNewParams{
		Spec: spec,
		Wait: param.NewOpt(true),
	})
	if err != nil {
		log.Fatalf("failed to create android instance: %s", err)
	}
	log.Printf("Created android instance with %s pre-installed in %s", asset.Name, time.Since(init))
	log.Printf("Connection URL: %s", instance.Status.EndpointWebSocketURL)
	log.Printf("Connection token: %s", instance.Status.Token)
}
