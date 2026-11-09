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
		out := flushWriter{writer: w}
		if err := runBuild(r.Context(), out); err != nil {
			fmt.Fprintf(out, "Error: %v\n", err)
		}
	})
	fmt.Printf("Simulator: https://console.limrun.com/stream/%s\nFiles are auto-synced. Build: curl http://localhost:3000/xcodebuild\nOptional MCP adapter: npm --prefix examples/ios-with-xcode run mcp (port 3001)\n", instance.Metadata.ID)
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
