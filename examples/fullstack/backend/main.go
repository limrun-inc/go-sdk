package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
	"github.com/limrun-inc/go-sdk/websocket/android"
	"github.com/limrun-inc/go-sdk/websocket/ios"
)

func main() { exampleutil.Main(run) }

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
		if err := exampleutil.Decode(w, r, &body); err != nil {
			exampleutil.Error(w, 400, err)
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
				exampleutil.Error(w, 500, err)
				return
			}
			if withExpo {
				client, err := api.IosInstances.CreateClient(*instance)
				if err != nil {
					exampleutil.Error(w, 500, err)
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
					exampleutil.Error(w, 500, err)
					return
				}
				retain(instance.Metadata.ID, client)
			}
			exampleutil.JSON(w, 200, map[string]string{
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
				exampleutil.Error(w, 500, err)
				return
			}
			if withExpo {
				client, err := api.AndroidInstances.CreateClient(*instance)
				if err != nil {
					exampleutil.Error(w, 500, err)
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
					exampleutil.Error(w, 500, err)
					return
				}
				retain(instance.Metadata.ID, client)
			}
			exampleutil.JSON(w, 200, map[string]string{
				"id":        instance.Metadata.ID,
				"webrtcUrl": instance.Status.EndpointWebSocketURL,
				"token":     instance.Status.Token,
			})
		default:
			exampleutil.Error(w, 400, fmt.Errorf("platform must be ios or android"))
		}
	})
	mux.HandleFunc("POST /stop-instance", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			InstanceID string `json:"instanceId"`
			Platform   string `json:"platform"`
		}
		if err := exampleutil.Decode(w, r, &body); err != nil {
			exampleutil.Error(w, 400, err)
			return
		}
		if body.InstanceID == "" {
			exampleutil.Error(w, 400, fmt.Errorf("instanceId is required"))
			return
		}
		var err error
		switch body.Platform {
		case "ios":
			err = api.IosInstances.Delete(r.Context(), body.InstanceID)
		case "", "android":
			err = api.AndroidInstances.Delete(r.Context(), body.InstanceID)
		default:
			exampleutil.Error(w, 400, fmt.Errorf("platform must be ios or android"))
			return
		}
		if err != nil {
			exampleutil.Error(w, 500, err)
			return
		}
		mu.Lock()
		client := clients[body.InstanceID]
		delete(clients, body.InstanceID)
		mu.Unlock()
		if client != nil {
			client.Close()
		}
		exampleutil.JSON(w, 200, map[string]string{"status": "success", "message": "Instance stopped successfully"})
	})
	fmt.Println("Backend: http://localhost:3000; start the frontend and open http://localhost:5173")
	return exampleutil.Serve(ctx, &http.Server{Addr: "127.0.0.1:3000", Handler: exampleutil.CORS(mux)})
}
