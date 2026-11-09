package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
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
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.IosInstanceNewParamsMetadata{Labels: map[string]string{"name": "appium-ios-example"}},
		Spec: limrun.IosInstanceNewParamsSpec{InitialAssets: []limrun.IosInstanceNewParamsSpecInitialAsset{{
			Kind: "App", Source: "URL", LaunchMode: "ForegroundIfRunning",
			URL: limrun.String("https://github.com/appium/WebDriverAgent/releases/download/v10.4.5/WebDriverAgentRunner-Build-Sim-arm64.zip"),
		}}},
	})
	if err != nil {
		return err
	}
	client, err := api.IosInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	info, err := client.DeviceInfo(ctx)
	if err != nil {
		return err
	}
	if instance.Status.TargetHTTPPortURLPrefix == "" {
		return fmt.Errorf("instance has no target HTTP port URL prefix")
	}
	wda, err := url.Parse(instance.Status.TargetHTTPPortURLPrefix)
	if err != nil {
		return err
	}
	// Appium defaults to 4444 without an explicit port, even for HTTPS.
	port := wda.Port()
	if port == "" {
		port = "443"
	}
	wda.Host = net.JoinHostPort(wda.Hostname(), port)
	wda.Path += "8100"
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, wda.String()+"/status", nil)
	if err != nil {
		cancel()
		return err
	}
	req.Header.Set("Authorization", "Bearer "+instance.Status.Token)
	response, checkErr := http.DefaultClient.Do(req)
	if response != nil {
		response.Body.Close()
	}
	cancel()
	if checkErr != nil || response.StatusCode >= 400 {
		cmd := client.Simctl(ctx, "launch", "booted", "com.facebook.WebDriverAgentRunner.xctrunner")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	if err := client.Close(); err != nil {
		return err
	}
	fmt.Printf("Device %s; instance %s remains available after the test.\n", info.UDID, instance.Metadata.ID)
	return runNode(ctx, "examples/appium-ios", "runner.mjs", map[string]string{
		"udid": info.UDID, "apiUrl": instance.Status.APIURL, "token": instance.Status.Token, "wdaUrl": wda.String(),
	})
}

// runNode sends instance credentials through stdin, not command-line arguments.
func runNode(ctx context.Context, directory, script string, input any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Dir = directory
	cmd.Stdin = strings.NewReader(string(data))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
