package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/build"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
)

func main() {
	flag.Parse()
	exampleutil.Main(run)
}

func run(ctx context.Context) error {
	if flag.NArg() != 1 {
		return fmt.Errorf("usage: go run ./examples/ios-with-xcode /path/to/project")
	}
	api := limrun.NewClient()
	sandbox, err := api.XcodeInstances.New(ctx, limrun.XcodeInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.XcodeInstanceNewParamsMetadata{Labels: map[string]string{"name": "ios-with-xcode-example"}},
	})
	if err != nil {
		return err
	}
	instance, err := api.IosInstances.New(ctx, limrun.IosInstanceNewParams{
		Wait:          limrun.Bool(true),
		ReuseIfExists: limrun.Bool(true),
		Metadata:      limrun.IosInstanceNewParamsMetadata{Labels: map[string]string{"name": "ios-with-xcode-example"}},
	})
	if err != nil {
		return err
	}
	xcode, err := api.XcodeInstances.CreateClient(*sandbox)
	if err != nil {
		return err
	}
	if _, err := xcode.AttachSimulator(ctx, instance.Status.APIURL, instance.Status.Token); err != nil {
		return err
	}
	synced, err := xcode.Sync(ctx, flag.Arg(0), nil)
	if err != nil {
		return err
	}
	if synced.StopWatching != nil {
		defer synced.StopWatching()
	}
	var buildMu sync.Mutex
	runBuild := func(ctx context.Context, out io.Writer) error {
		buildMu.Lock()
		defer buildMu.Unlock()
		client, err := api.XcodeInstances.CreateClient(*sandbox, build.WithExecutionOptions(build.ExecOptions{Command: out, Stdout: out, Stderr: out}))
		if err != nil {
			return err
		}
		process, err := client.Xcodebuild(ctx, nil, nil)
		if err != nil {
			return err
		}
		result, err := process.Wait(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Build finished with exit code %d\n", result.ExitCode)
		if result.ExitCode != 0 {
			return fmt.Errorf("xcodebuild failed: %d", result.ExitCode)
		}
		return nil
	}
	if err := runBuild(ctx, os.Stdout); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /xcodebuild", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		out := exampleutil.FlushWriter{Writer: w}
		if err := runBuild(r.Context(), out); err != nil {
			fmt.Fprintf(out, "Error: %v\n", err)
		}
	})
	fmt.Printf("Simulator: https://console.limrun.com/stream/%s\nFiles are auto-synced. Build: curl http://localhost:3000/xcodebuild\nOptional MCP adapter: npm --prefix examples/ios-with-xcode run mcp (port 3001)\n", instance.Metadata.ID)
	return exampleutil.Serve(ctx, &http.Server{Addr: "127.0.0.1:3000", Handler: mux})
}
