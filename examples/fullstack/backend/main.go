package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/websocket/android"
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
	api := limrun.NewClient()
	var mu sync.Mutex
	// Retain control connections so app-exit callbacks keep firing after POST returns.
	clients := map[string]io.Closer{}
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		for _, client := range clients {
			client.Close()
		}
	}()
	retain := func(id string, client io.Closer) {
		mu.Lock()
		defer mu.Unlock()
		if old := clients[id]; old != nil {
			old.Close()
		}
		clients[id] = client
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /create-instance", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			WebSessionID string `json:"webSessionId"`
			Platform     string `json:"platform"`
			IOSModel     string `json:"iosModel"`
			WithExpoGo54 *bool  `json:"withExpoGo54"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, 400, err)
			return
		}
		if body.Platform == "" {
			body.Platform = "android"
		}
		if body.IOSModel == "" {
			body.IOSModel = "iphone"
		}
		withExpo := body.WithExpoGo54 == nil || *body.WithExpoGo54
		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
		useClue := clientIP != "" && clientIP != "::1" && clientIP != "127.0.0.1"
		labels := map[string]string{}
		if body.WebSessionID != "" {
			labels["webSessionId"] = body.WebSessionID
		}
		switch body.Platform {
		case "ios":
			spec := limrun.IosInstanceNewParamsSpec{Model: body.IOSModel}
			if withExpo {
				spec.InitialAssets = []limrun.IosInstanceNewParamsSpecInitialAsset{
					{Kind: "App", Source: "AssetName", AssetName: limrun.String("appstore/Expo-Go-54.0.6.tar.gz")},
				}
			}
			if useClue {
				spec.Clues = []limrun.IosInstanceNewParamsSpecClue{{Kind: "ClientIP", ClientIP: limrun.String(clientIP)}}
			}
			instance, err := api.IosInstances.New(r.Context(), limrun.IosInstanceNewParams{
				Wait:          limrun.Bool(true),
				ReuseIfExists: limrun.Bool(true),
				Spec:          spec,
				Metadata:      limrun.IosInstanceNewParamsMetadata{Labels: labels},
			})
			if err != nil {
				writeError(w, 500, err)
				return
			}
			if withExpo {
				client, err := api.IosInstances.CreateClient(*instance)
				if err != nil {
					writeError(w, 500, err)
					return
				}
				// Use the server context: monitoring outlives the create request.
				err = client.LaunchApp(ctx, "host.exp.Exponent", ios.WithLaunchMode(ios.LaunchModeRelaunchIfRunning), ios.WithAppExit(func(lines []string, info ios.AppExitInfo) {
					log.Printf("Expo Go exited: %s", info.Reason)
					if info.Crash != nil {
						log.Printf("%s\n%s", info.Crash.LongMsg, info.Crash.StackTrace)
					}
					for _, line := range lines {
						log.Print(line)
					}
				}))
				if err != nil {
					client.Close()
					writeError(w, 500, err)
					return
				}
				retain(instance.Metadata.ID, client)
			}
			writeJSON(w, 200, map[string]string{
				"id":        instance.Metadata.ID,
				"webrtcUrl": instance.Status.EndpointWebSocketURL,
				"token":     instance.Status.Token,
			})
		case "android":
			spec := limrun.AndroidInstanceNewParamsSpec{}
			if withExpo {
				spec.InitialAssets = []limrun.AndroidInstanceNewParamsSpecInitialAsset{
					{Kind: "App", Source: "AssetName", AssetName: limrun.String("appstore/Expo-Go-54.0.6.apk")},
				}
			}
			if useClue {
				spec.Clues = []limrun.AndroidInstanceNewParamsSpecClue{{Kind: "ClientIP", ClientIP: limrun.String(clientIP)}}
			}
			instance, err := api.AndroidInstances.New(r.Context(), limrun.AndroidInstanceNewParams{
				Wait:     limrun.Bool(true),
				Spec:     spec,
				Metadata: limrun.AndroidInstanceNewParamsMetadata{Labels: labels},
			})
			if err != nil {
				writeError(w, 500, err)
				return
			}
			if withExpo {
				client, err := api.AndroidInstances.CreateClient(*instance)
				if err != nil {
					writeError(w, 500, err)
					return
				}
				_, err = client.LaunchApp(ctx, "host.exp.exponent", &android.LaunchAppOptions{Mode: android.RelaunchIfRunning, OnExit: func(lines []string, info android.AppExitInfo) {
					log.Printf("Expo Go exited: %s", info.Reason)
					if info.Crash != nil {
						log.Print(info.Crash.StackTrace)
					}
					for _, line := range lines {
						log.Print(line)
					}
				}})
				if err != nil {
					client.Close()
					writeError(w, 500, err)
					return
				}
				retain(instance.Metadata.ID, client)
			}
			writeJSON(w, 200, map[string]string{
				"id":        instance.Metadata.ID,
				"webrtcUrl": instance.Status.EndpointWebSocketURL,
				"token":     instance.Status.Token,
			})
		default:
			writeError(w, 400, fmt.Errorf("platform must be ios or android"))
		}
	})
	mux.HandleFunc("POST /stop-instance", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			InstanceID string `json:"instanceId"`
			Platform   string `json:"platform"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, 400, err)
			return
		}
		if body.InstanceID == "" {
			writeError(w, 400, fmt.Errorf("instanceId is required"))
			return
		}
		var err error
		switch body.Platform {
		case "ios":
			err = api.IosInstances.Delete(r.Context(), body.InstanceID)
		case "", "android":
			err = api.AndroidInstances.Delete(r.Context(), body.InstanceID)
		default:
			writeError(w, 400, fmt.Errorf("platform must be ios or android"))
			return
		}
		if err != nil {
			writeError(w, 500, err)
			return
		}
		mu.Lock()
		client := clients[body.InstanceID]
		delete(clients, body.InstanceID)
		mu.Unlock()
		if client != nil {
			client.Close()
		}
		writeJSON(w, 200, map[string]string{"status": "success", "message": "Instance stopped successfully"})
	})
	fmt.Println("Backend: http://localhost:3000; start the frontend and open http://localhost:5173")
	return serve(ctx, &http.Server{Addr: "127.0.0.1:3000", Handler: cors(mux)})
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"status": "error", "message": err.Error()})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(value)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
