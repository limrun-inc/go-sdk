package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
)

func TestDeviceSessionGrantsOnlyCompletedBuildAsset(t *testing.T) {
	var mu sync.Mutex
	var permissions []string
	assetName := ""
	uploaded := false
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/access_tokens":
			var body struct {
				Permissions []string `json:"permissions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			permissions = body.Permissions
			fmt.Fprint(w, `{"token":"access","expiresAt":"2100-01-01T00:00:00Z"}`)
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
	store := testStore(t)
	mustPut(t, store, "appleCertificate", "TEAM/DISTRIBUTION", map[string]string{"serialNumber": "SERIAL", "certificateP12Base64": "Y2VydA=="})
	mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", map[string]string{
		"teamID":                    "TEAM",
		"bundleIDs":                 "com.example.app",
		"certificateSerialNumbers":  "SERIAL",
		"deviceIDs":                 `[{"udid":"abcd"}]`,
		"provisioningProfileBase64": "cHJvZmlsZQ==",
	})
	jobs := newInstalls()
	defer jobs.close()
	handler := newHandler(context.Background(), api, store, jobs, "https://registry.example")
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
		return w
	}
	granted := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), permissions...)
	}
	if w := post("/session", ""); w.Code != 200 {
		t.Fatalf("relay session: %d %s", w.Code, w.Body.String())
	}
	if got := granted(); !reflect.DeepEqual(got, []string{"applerelay:*:connect"}) {
		t.Fatalf("relay permissions: %v", got)
	}
	if w := post("/device-session", "{}"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if got := granted(); !reflect.DeepEqual(got, []string{"device:*:install"}) {
		t.Fatalf("initial device permissions: %v", got)
	}
	if w := post("/device-session", `{"installId":"unknown"}`); w.Code != 404 {
		t.Fatalf("unknown install: %d", w.Code)
	}
	capture := installFakeCLI(t)
	body, _ := json.Marshal(installRequest{
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
	_, token, _ := strings.Cut(argValue(capturedArgs(t, capture), "--webhook-header"), "=")
	jobs.receive(token, json.RawMessage(`{"status":"SUCCEEDED"}`))
	status, _ := jobs.get(accepted.InstallID)
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
	if got := granted(); !reflect.DeepEqual(got, []string{"device:*:install", "asset:exact:read"}) {
		t.Fatalf("overbroad or wrong asset grant: %v", got)
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["assetId"] != "exact" || result["assetName"] != assetName {
		t.Fatalf("session lost asset identity: %v", result)
	}
}
