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
	store, err := storebackend.NewSecretStore("examples/device-install/backend/.secrets")
	if err != nil {
		return err
	}
	jobs := storebackend.NewJobs()
	defer jobs.Close()
	registryURL := os.Getenv("LIM_REGISTRY_ENDPOINT")
	if registryURL == "" {
		registryURL = "https://registry.limrun.com"
	}
	return storebackend.Serve(ctx, newHandler(ctx, api, store, jobs, registryURL), jobs.WebhookHandler("X-Install-Token"))
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
	mux.HandleFunc("POST /device-session", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			InstallID string `json:"installId"`
		}
		if err := exampleutil.Decode(w, r, &body); err != nil {
			exampleutil.Error(w, 400, err)
			return
		}
		// Initially grant device installation only. Asset read permission is added
		// only for the exact completed build selected by the install ID.
		scopes := []string{"device:*:install"}
		result := map[string]any{"registryUrl": registryURL}
		if body.InstallID != "" {
			status, ok := jobs.Get(body.InstallID)
			if !ok {
				exampleutil.Error(w, 404, fmt.Errorf("install build not found"))
				return
			}
			if status.State != "succeeded" {
				exampleutil.Error(w, 409, fmt.Errorf("the install build must finish successfully first"))
				return
			}
			assets, err := api.Assets.List(r.Context(), limrun.AssetListParams{NameFilter: limrun.String(status.AssetName)})
			if err != nil {
				exampleutil.Error(w, 500, err)
				return
			}
			assetID := ""
			for _, asset := range *assets {
				if asset.Name == status.AssetName && asset.Md5 != "" {
					assetID = asset.ID
					break
				}
			}
			if assetID == "" {
				exampleutil.Error(w, 404, fmt.Errorf("uploaded asset %s is not available yet", status.AssetName))
				return
			}
			scopes = append(scopes, "asset:"+assetID+":read")
			result["assetId"], result["assetName"] = assetID, status.AssetName
		}
		session, err := api.ScopedTokens.New(r.Context(), limrun.ScopedTokenNewParams{Scopes: scopes})
		if err != nil {
			exampleutil.Error(w, 500, err)
			return
		}
		result["token"], result["expiresAt"] = session.Token, session.ExpiresAt
		exampleutil.JSON(w, 200, result)
	})
	mux.HandleFunc("POST /install", func(w http.ResponseWriter, r *http.Request) {
		var body storebackend.Request
		if err := exampleutil.Decode(w, r, &body); err != nil {
			exampleutil.Error(w, 400, err)
			return
		}
		if err := body.Validate("install"); err != nil {
			exampleutil.Error(w, 400, err)
			return
		}
		id, err := jobs.Start(ctx, store, "install", body)
		if err != nil {
			exampleutil.Error(w, 500, err)
			return
		}
		exampleutil.JSON(w, 202, map[string]string{"installId": id})
	})
	mux.HandleFunc("GET /install/{id}", func(w http.ResponseWriter, r *http.Request) {
		status, ok := jobs.Get(r.PathValue("id"))
		if !ok {
			exampleutil.Error(w, 404, fmt.Errorf("install build not found"))
			return
		}
		exampleutil.JSON(w, 200, status)
	})
	return mux
}
