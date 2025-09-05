package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/limrun-inc/go-sdk/api"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Need to provide a path to a file to run")
	}
	apkPath := os.Args[1]
	token := os.Getenv("LIM_TOKEN") // lim_yourtoken
	limrun := api.NewDefaultClient(token)

	ctx := context.TODO()
	initUpl := time.Now()
	asset, err := limrun.PutAndUploadAsset(ctx, apkPath)
	if err != nil {
		log.Fatalf("failed to upload asset to limrun: %s", err)
	}
	log.Printf("Uploaded %s in %s", apkPath, time.Since(initUpl))
	body := &api.AndroidInstanceCreate{
		Spec: api.NewOptAndroidInstanceCreateSpec(api.AndroidInstanceCreateSpec{
			InitialAssets: []api.AndroidInstanceCreateSpecInitialAssetsItem{
				{
					Kind:   api.AndroidInstanceCreateSpecInitialAssetsItemKindApp,
					Source: api.AndroidInstanceCreateSpecInitialAssetsItemSourceAssetName,

					// You can use "path.Base(filePath)" as well since PutAndUploadAsset derives the name
					// from file name that it uploads.
					AssetName: api.NewOptString(asset.Name),
				},
			},
		}),
	}
	init := time.Now()
	instance, err := limrun.CreateAndroidInstance(ctx, body, api.CreateAndroidInstanceParams{
		Wait: api.NewOptBool(true),
	})
	if err != nil {
		log.Fatalf("failed to create android instance: %s", err)
	}
	log.Printf("Created android instance with %s pre-installed in %s", asset.Name, time.Since(init))
	log.Printf("Connection URL: %s", instance.Status.EndpointWebSocketUrl.Value)
	log.Printf("Connection token: %s", instance.Status.Token)
}
