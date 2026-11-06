package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/examples/internal/storebackend"
	"github.com/limrun-inc/go-sdk/option"
)

func TestDeviceSessionGrantsOnlyCompletedBuildAsset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI uses a POSIX shell")
	}
	var mu sync.Mutex
	var scopes []string
	assetName := ""
	uploaded := false
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/scoped_tokens":
			var body struct {
				Scopes []string `json:"scopes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			scopes = body.Scopes
			fmt.Fprint(w, `{"token":"scoped","expiresAt":"2100-01-01T00:00:00Z"}`)
		case "/v1/assets":
			// Include a partial-name match: the backend must select the exact uploaded asset.
			md5 := ""
			if uploaded {
				md5 = "abcd"
			}
			json.NewEncoder(w).Encode([]map[string]string{
				{"id": "wrong", "name": assetName + "-other", "md5": "abcd"},
				{"id": "exact", "name": assetName, "md5": md5},
			})
		default:
			t.Errorf("unexpected SDK route: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer service.Close()
	api := limrun.NewClient(option.WithAPIKey("test"), option.WithBaseURL(service.URL), option.WithMaxRetries(0))
	t.Setenv("SECRETS_DIR", "")
	store, err := storebackend.NewSecretStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []storebackend.Secret{
		{
			Type: "appleCertificate",
			Name: "TEAM/DISTRIBUTION",
			Data: map[string]string{"serialNumber": "SERIAL", "certificateP12Base64": "Y2VydA=="},
		},
		{
			Type: "appleProvisioningProfile",
			Name: "TEAM/PROFILE",
			Data: map[string]string{
				"teamID":                    "TEAM",
				"bundleIDs":                 "com.example.app",
				"certificateSerialNumbers":  "SERIAL",
				"deviceIDs":                 `[{"udid":"abcd"}]`,
				"provisioningProfileBase64": "cHJvZmlsZQ==",
			},
		},
	} {
		if _, err := store.Put(secret.Type, secret.Name, secret.Data, ""); err != nil {
			t.Fatal(err)
		}
	}
	jobs := storebackend.NewJobs()
	defer jobs.Close()
	handler := newHandler(context.Background(), api, store, jobs, "https://registry.example")
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
		return w
	}
	if w := post("/session", ""); w.Code != 200 {
		t.Fatalf("relay session: %d %s", w.Code, w.Body.String())
	}
	mu.Lock()
	got := append([]string(nil), scopes...)
	mu.Unlock()
	if !reflect.DeepEqual(got, []string{"applerelay:*:connect"}) {
		t.Fatalf("relay scopes: %v", got)
	}
	if w := post("/device-session", "{}"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	mu.Lock()
	got = append([]string(nil), scopes...)
	mu.Unlock()
	if !reflect.DeepEqual(got, []string{"device:*:install"}) {
		t.Fatalf("initial device scopes: %v", got)
	}
	if w := post("/device-session", `{"installId":"unknown"}`); w.Code != 404 {
		t.Fatalf("unknown install: %d", w.Code)
	}
	bin := t.TempDir()
	capture := filepath.Join(bin, "args")
	if err := os.WriteFile(filepath.Join(bin, "lim"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LIMRUN_EXAMPLE_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIMRUN_EXAMPLE_ARGS", capture)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	body, _ := json.Marshal(storebackend.Request{
		ProjectPath: t.TempDir(),
		TeamID:      "TEAM",
		BundleID:    "com.example.app",
		DeviceUDID:  "AB-CD",
		WebhookURL:  "https://hooks.example",
	})
	w := post("/install", string(body))
	if w.Code != 202 {
		t.Fatalf("install: %d %s", w.Code, w.Body.String())
	}
	var accepted struct {
		InstallID string `json:"installId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	sessionBody := fmt.Sprintf(`{"installId":%q}`, accepted.InstallID)
	if w := post("/device-session", sessionBody); w.Code != 409 {
		t.Fatalf("running install: %d", w.Code)
	}
	var token string
	deadline := time.Now().Add(5 * time.Second)
	for token == "" {
		data, _ := os.ReadFile(capture)
		for _, arg := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(arg, "X-Install-Token=") {
				token = strings.TrimPrefix(arg, "X-Install-Token=")
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("fake CLI did not receive webhook token")
		}
		if token == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	jobs.Receive(token, json.RawMessage(`{"status":"SUCCEEDED"}`))
	status, _ := jobs.Get(accepted.InstallID)
	mu.Lock()
	assetName = status.AssetName
	mu.Unlock()
	if w := post("/device-session", sessionBody); w.Code != 404 {
		t.Fatalf("unfinished asset upload: %d", w.Code)
	}
	mu.Lock()
	uploaded = true
	mu.Unlock()
	w = post("/device-session", sessionBody)
	if w.Code != 200 {
		t.Fatalf("completed session: %d %s", w.Code, w.Body.String())
	}
	mu.Lock()
	got = append([]string(nil), scopes...)
	mu.Unlock()
	if !reflect.DeepEqual(got, []string{"device:*:install", "asset:exact:read"}) {
		t.Fatalf("overbroad or wrong asset grant: %v", got)
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["assetId"] != "exact" || result["assetName"] != assetName {
		t.Fatalf("session lost asset identity: %v", result)
	}
}
