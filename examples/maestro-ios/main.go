package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/websocket/ios"
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
	defer cleanup(func(ctx context.Context) error { return api.IosInstances.Delete(ctx, instance.Metadata.ID) })
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
			return fmt.Errorf("maestro runner did not become ready")
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
	cmd.Env = withEnv(prepared.Env)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	testErr := cmd.Run()
	// Save the recording even when the test runner fails or Ctrl+C is pressed.
	var recordingErr error
	cleanup(func(ctx context.Context) error {
		_, recordingErr = client.StopRecording(ctx, ios.SaveRecordingTo{LocalPath: "examples/maestro-ios/video.mp4"})
		return recordingErr
	})
	return errors.Join(testErr, recordingErr)
}

// cleanup still runs when Ctrl+C has canceled the example's main context.
func cleanup(fn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := fn(ctx); err != nil {
		log.Printf("cleanup: %v", err)
	}
}

func withEnv(overrides map[string]string) []string {
	env := os.Environ()
	for key, value := range overrides {
		prefix := key + "="
		for i := len(env) - 1; i >= 0; i-- {
			if strings.HasPrefix(env[i], prefix) {
				env = append(env[:i], env[i+1:]...)
			}
		}
		env = append(env, prefix+value)
	}
	return env
}
