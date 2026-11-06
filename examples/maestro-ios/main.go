package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
	"github.com/limrun-inc/go-sdk/websocket/ios"
)

func main() { exampleutil.Main(run) }

func run(ctx context.Context) error {
	expoURL := os.Getenv("EXPO_URL")
	if expoURL == "" {
		return fmt.Errorf("set EXPO_URL to your running Expo app")
	}
	api := limrun.NewClient()
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.IosInstanceNewParamsMetadata{Labels: map[string]string{"name": "maestro-ios-example"}},
		Spec: limrun.IosInstanceNewParamsSpec{InitialAssets: []limrun.IosInstanceNewParamsSpecInitialAsset{
			{Kind: "App", Source: "AssetName", AssetName: limrun.String("appstore/Expo-Go-54.0.6.tar.gz")},
			{Kind: "App", Source: "AssetName", AssetName: limrun.String("appstore/maestro-ios-runner-2.5.1.tar.gz")},
		}},
	})
	if err != nil {
		return err
	}
	defer exampleutil.Cleanup(func(ctx context.Context) error { return api.IosInstances.Delete(ctx, instance.Metadata.ID) })
	client, err := api.IosInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	defer client.Close()
	runnerURL := instance.Status.TargetHTTPPortURLPrefix + strconv.Itoa(ios.MaestroRunnerPort)
	if !ios.IsMaestroRunnerRunning(ctx, runnerURL, instance.Status.Token) {
		cmd := client.Simctl(ctx, "launch", "--terminate-running-process", "booted", ios.MaestroRunnerBundleID)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		if !ios.WaitForMaestroRunner(ctx, runnerURL, instance.Status.Token) {
			return fmt.Errorf("Maestro runner did not become ready")
		}
	}
	prepared, err := ios.PrepareMaestroRun(ctx, client, runnerURL)
	if err != nil {
		return err
	}
	fmt.Printf("Maestro %s; open https://console.limrun.com/stream/%s\n", prepared.MaestroVersion, instance.Metadata.ID)
	if err := client.StartRecording(ctx, nil); err != nil {
		return err
	}
	args := append([]string{"test"}, prepared.Args...)
	args = append(args, "--test-output-dir", "artifacts", filepath.Join("flows", "expo-sample.yaml"))
	command := "maestro"
	if runtime.GOOS == "windows" {
		command = "cmd"
		args = append([]string{"/c", "maestro"}, args...)
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = "examples/maestro-ios"
	prepared.Env["MAESTRO_EXPO_URL"] = expoURL
	cmd.Env = exampleutil.Env(prepared.Env)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	testErr := cmd.Run()
	// Save the recording even when the test runner fails or Ctrl+C is pressed.
	var recordingErr error
	exampleutil.Cleanup(func(ctx context.Context) error {
		_, recordingErr = client.StopRecording(ctx, ios.SaveRecordingTo{LocalPath: "examples/maestro-ios/video.mp4"})
		return recordingErr
	})
	return errors.Join(testErr, recordingErr)
}
