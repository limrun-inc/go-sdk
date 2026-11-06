package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
	"github.com/limrun-inc/go-sdk/examples/internal/storebackend"
)

func main() { exampleutil.Main(run) }

func run(ctx context.Context) error {
	api := limrun.NewClient()
	store, err := storebackend.NewSecretStore("examples/publish-to-stores/backend/.secrets")
	if err != nil {
		return err
	}
	jobs := storebackend.NewJobs()
	defer jobs.Close()
	registryURL := os.Getenv("LIM_REGISTRY_ENDPOINT")
	if registryURL == "" {
		registryURL = "https://registry.limrun.com"
	}
	return storebackend.Serve(ctx, newHandler(ctx, api, store, jobs, registryURL), jobs.WebhookHandler("X-Publish-Token"))
}

func newHandler(ctx context.Context, api limrun.Client, store *storebackend.SecretStore, jobs *storebackend.Jobs, registryURL string) http.Handler {
	mux := http.NewServeMux()
	store.Register(mux)
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		session, err := api.ScopedTokens.New(r.Context(), limrun.ScopedTokenNewParams{Scopes: []string{"applerelay:*:connect"}})
		if err != nil {
			exampleutil.Error(w, 500, err)
			return
		}
		exampleutil.JSON(w, 200, map[string]any{
			"token":       session.Token,
			"expiresAt":   session.ExpiresAt,
			"registryUrl": registryURL,
			"secretsDir":  store.Directory,
		})
	})
	for _, route := range []struct{ path, kind string }{{"POST /publish", "publish"}, {"POST /publish/android", "android"}} {
		kind := route.kind
		mux.HandleFunc(route.path, func(w http.ResponseWriter, r *http.Request) {
			var body storebackend.Request
			if err := exampleutil.Decode(w, r, &body); err != nil {
				exampleutil.Error(w, 400, err)
				return
			}
			if err := body.Validate(kind); err != nil {
				exampleutil.Error(w, 400, err)
				return
			}
			id, err := jobs.Start(ctx, store, kind, body)
			if err != nil {
				exampleutil.Error(w, 500, err)
				return
			}
			exampleutil.JSON(w, 202, map[string]string{"publishId": id})
		})
	}
	mux.HandleFunc("GET /publish/{id}", func(w http.ResponseWriter, r *http.Request) {
		status, ok := jobs.Get(r.PathValue("id"))
		if !ok {
			exampleutil.Error(w, 404, fmt.Errorf("publish not found"))
			return
		}
		exampleutil.JSON(w, 200, status)
	})
	mux.HandleFunc("POST /project/android-package", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ProjectPath string `json:"projectPath"`
		}
		if err := exampleutil.Decode(w, r, &body); err != nil {
			exampleutil.Error(w, 400, err)
			return
		}
		if body.ProjectPath == "" {
			exampleutil.Error(w, 400, fmt.Errorf("projectPath is required"))
			return
		}
		var value any
		if name := storebackend.DetectAndroidPackage(body.ProjectPath); name != "" {
			value = name
		}
		exampleutil.JSON(w, 200, map[string]any{"packageName": value})
	})
	return mux
}
