package main

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Need to provide path to APK file or directory that contains APK files")
	}
	givenPath := os.Args[1]
	token := os.Getenv("LIM_API_KEY") // lim_yourtoken
	lim := limrun.NewClient(option.WithAPIKey(token))
	ctx := context.TODO()

	// Figure out all the file paths.
	var apkPaths []string
	if err := filepath.WalkDir(givenPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".apk") {
			return nil
		}
		log.Printf("Will install %s\n", d.Name())
		apkPaths = append(apkPaths, path)
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	// Make sure all files are uploaded.
	var names []string
	var wg sync.WaitGroup
	for _, apkPath := range apkPaths {
		wg.Add(1)
		initUpl := time.Now()
		go func() {
			asset, err := lim.Assets.GetOrUpload(ctx, limrun.AssetGetOrUploadParams{
				Path: apkPath,
			})
			if err != nil {
				log.Fatalf("failed to upload asset to limrun: %s", err)
			}
			names = append(names, asset.Name)
			log.Printf("Uploaded %s in %s", apkPath, time.Since(initUpl))
			wg.Done()
		}()
	}
	wg.Wait()

	// Create the instance with the uploaded assets.
	log.Printf("Using %d files", len(names))
	spec := limrun.AndroidInstanceNewParamsSpec{
		InitialAssets: []limrun.AndroidInstanceNewParamsSpecInitialAsset{
			{
				Kind:       "App",
				Source:     "AssetNames",
				AssetNames: names,
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
	log.Printf("Created android instance pre-installed %d APKs in %s", len(names), time.Since(init))
	log.Printf("Connection URL: %s", instance.Status.EndpointWebSocketURL)
	log.Printf("Connection token: %s", instance.Status.Token)
}
