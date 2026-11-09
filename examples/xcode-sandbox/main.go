package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/build"
)

func main() {
	flag.Parse()
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
	if flag.NArg() != 1 {
		return fmt.Errorf("usage: go run ./examples/xcode-sandbox /path/to/project")
	}
	api := limrun.NewClient()
	instance, err := api.XcodeInstances.New(ctx, limrun.XcodeInstanceNewParams{
		Wait:     limrun.Bool(true),
		Metadata: limrun.XcodeInstanceNewParamsMetadata{Labels: map[string]string{"name": "xcode-instance-example"}},
		Spec: limrun.XcodeInstanceNewParamsSpec{Role: limrun.XcodeInstanceNewParamsSpecRole{Permissions: []string{
			"asset:name/xcode-example/*:create", "asset:name/xcode-example/*:upload", "asset:name/xcode-example/*:update",
		}}},
	})
	if err != nil {
		return err
	}
	xcode, err := api.XcodeInstances.CreateClient(*instance)
	if err != nil {
		return err
	}
	synced, err := xcode.Sync(ctx, flag.Arg(0), nil)
	if err != nil {
		return err
	}
	if synced.StopWatching != nil {
		defer synced.StopWatching()
	}
	// Use one build at a time, with writers belonging to that request.
	var buildMu sync.Mutex
	runBuild := func(ctx context.Context, out io.Writer, sdk, target string) error {
		buildMu.Lock()
		defer buildMu.Unlock()
		client, err := api.XcodeInstances.CreateClient(*instance, build.WithExecutionOptions(build.ExecOptions{Command: out, Stdout: out, Stderr: out}))
		if err != nil {
			return err
		}
		settings := &build.XcodeProjectConfig{}
		if sdk != "" {
			settings.SDK = &sdk
		}
		options := &build.XcodeBuildOptions{}
		if target != "" {
			options.Upload = &build.BuildUpload{Target: &target}
		}
		process, err := client.Xcodebuild(ctx, settings, options)
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
		if target != "" {
			// Download authorization belongs to the caller, separately from workload upload grants.
			name := strings.TrimPrefix(target, "name/")
			assets, err := api.Assets.List(ctx, limrun.AssetListParams{NameFilter: limrun.String(name), IncludeDownloadURL: limrun.Bool(true)})
			if err != nil {
				return err
			}
			for _, asset := range *assets {
				if asset.Name == name && asset.SignedDownloadURL != "" {
					fmt.Fprintf(out, "Build artifact download URL: %s\n", asset.SignedDownloadURL)
				}
			}
		}
		return nil
	}
	if err := runBuild(ctx, os.Stdout, "", ""); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /xcodebuild", func(w http.ResponseWriter, r *http.Request) {
		sdk, target := r.URL.Query().Get("sdk"), r.URL.Query().Get("target")
		if sdk != "" && sdk != "iphoneos" && sdk != "iphonesimulator" {
			http.Error(w, "sdk must be iphoneos or iphonesimulator", 400)
			return
		}
		if target != "" && (!strings.HasPrefix(target, "name/xcode-example/") || target == "name/xcode-example/") {
			http.Error(w, "Use a concrete target under name/xcode-example/.", 400)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		out := flushWriter{writer: w}
		if err := runBuild(r.Context(), out, sdk, target); err != nil {
			fmt.Fprintf(out, "Error: %v\n", err)
		}
	})
	fmt.Printf("Watching files on Xcode instance %s.\nBuild: http://localhost:3000/xcodebuild\nUpload: http://localhost:3000/xcodebuild?sdk=iphoneos&target=name/xcode-example/device-build.ipa\n", instance.Metadata.ID)
	return serve(ctx, &http.Server{Addr: "127.0.0.1:3000", Handler: mux})
}

func serve(ctx context.Context, server *http.Server) error {
	done := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		// Shut down on a fresh context: ctx is already canceled when this runs.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("cleanup: %v", err)
		}
	}()
	err := server.ListenAndServe()
	close(done)
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

type flushWriter struct{ writer http.ResponseWriter }

func (w flushWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if f, ok := w.writer.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}
