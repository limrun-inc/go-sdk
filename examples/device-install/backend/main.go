// The device-install backend keeps LIM_API_KEY server-side, mints access
// tokens for the browser, stores signing secrets as files, and runs one
// detached `lim xcode build` per install whose result arrives by webhook.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	store, err := newSecretStore("examples/device-install/backend/.secrets")
	if err != nil {
		return err
	}
	jobs := newInstalls()
	defer jobs.close()
	registryURL := os.Getenv("LIM_REGISTRY_ENDPOINT")
	if registryURL == "" {
		registryURL = "https://registry.limrun.com"
	}
	return serve(ctx, newHandler(ctx, api, store, jobs, registryURL), jobs.webhookHandler())
}

func newHandler(ctx context.Context, api limrun.Client, store *secretStore, jobs *installs, registryURL string) http.Handler {
	mux := http.NewServeMux()
	store.register(mux)
	// Apple credentials are created in the browser through the registry relay.
	// The long-lived API key stays here; this token can only open that relay.
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		session, err := api.AccessTokens.New(r.Context(), limrun.AccessTokenNewParams{Permissions: []string{"applerelay:*:connect"}})
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{
			"token":       session.Token,
			"expiresAt":   session.ExpiresAt,
			"registryUrl": registryURL,
			"secretsDir":  store.directory,
		})
	})
	mux.HandleFunc("POST /device-session", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			InstallID string `json:"installId"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, 400, err)
			return
		}
		// Initially grant device installation only. Asset read permission is added
		// only for the exact completed build selected by the install ID.
		permissions := []string{"device:*:install"}
		result := map[string]any{"registryUrl": registryURL}
		if body.InstallID != "" {
			status, ok := jobs.get(body.InstallID)
			if !ok {
				writeError(w, 404, fmt.Errorf("install build not found"))
				return
			}
			if status.State != "succeeded" {
				writeError(w, 409, fmt.Errorf("the install build must finish successfully first"))
				return
			}
			assets, err := api.Assets.List(r.Context(), limrun.AssetListParams{NameFilter: limrun.String(status.AssetName)})
			if err != nil {
				writeError(w, 500, err)
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
				writeError(w, 404, fmt.Errorf("uploaded asset %s is not available yet", status.AssetName))
				return
			}
			permissions = append(permissions, "asset:"+assetID+":read")
			result["assetId"], result["assetName"] = assetID, status.AssetName
		}
		session, err := api.AccessTokens.New(r.Context(), limrun.AccessTokenNewParams{Permissions: permissions})
		if err != nil {
			writeError(w, 500, err)
			return
		}
		result["token"], result["expiresAt"] = session.Token, session.ExpiresAt
		writeJSON(w, 200, result)
	})
	// The webhook URL comes from the UI with each request: limbuild runs inside
	// Limrun's cloud and rejects private or IP-literal callback URLs, so it must
	// be a public URL that forwards to the webhook receiver on port 3001.
	mux.HandleFunc("POST /install", func(w http.ResponseWriter, r *http.Request) {
		var body installRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, 400, err)
			return
		}
		if err := body.validate(); err != nil {
			writeError(w, 400, err)
			return
		}
		id, err := jobs.start(ctx, store, body)
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 202, map[string]string{"installId": id})
	})
	mux.HandleFunc("GET /install/{id}", func(w http.ResponseWriter, r *http.Request) {
		status, ok := jobs.get(r.PathValue("id"))
		if !ok {
			writeError(w, 404, fmt.Errorf("install build not found"))
			return
		}
		writeJSON(w, 200, status)
	})
	return mux
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

// The Vite dev server on port 5173 is the only expected browser origin.
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

// Only the webhook listener is exposed publicly; the main API stays on loopback
// because its routes act on local paths the UI provides.
func serve(ctx context.Context, api http.Handler, hooks http.Handler) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, 2)
	go func() { errs <- listen(ctx, &http.Server{Addr: "127.0.0.1:3000", Handler: cors(api)}) }()
	go func() { errs <- listen(ctx, &http.Server{Addr: ":3001", Handler: hooks}) }()
	log.Print("Local API: http://localhost:3000; webhook receiver: port 3001")
	first := <-errs
	cancel()
	second := <-errs
	if first != nil {
		return first
	}
	return second
}

// listen serves until ctx is canceled, then drains connections for up to 30 seconds.
func listen(ctx context.Context, server *http.Server) error {
	done := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
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
