package storebackend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// The subprocess stands in for lim: no cloud build or store publish is performed.
func TestMain(m *testing.M) {
	if capture := os.Getenv("EXAMPLE_CLI_CAPTURE"); capture != "" {
		data, _ := json.Marshal(map[string]any{
			"args":             os.Args[1:],
			"keystorePassword": os.Getenv("LIM_KEYSTORE_PASSWORD"),
			"keyPassword":      os.Getenv("LIM_KEY_PASSWORD"),
			"playToken":        os.Getenv("LIM_PLAYSTORE_ACCESS_TOKEN"),
		})
		if err := os.WriteFile(capture, data, 0600); err != nil {
			os.Exit(1)
		}
		os.Stdout.WriteString(`{"consoleUrl":"https://console.example/build"}`)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func mustPut(t *testing.T, s *SecretStore, kind, name string, data map[string]string) {
	t.Helper()
	if _, err := s.Put(kind, name, data, ""); err != nil {
		t.Fatal(err)
	}
}
func testStore(t *testing.T) *SecretStore {
	t.Helper()
	t.Setenv("SECRETS_DIR", "")
	s, err := NewSecretStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestSecretStoreIsCompatibleWithEncodedNamesAndHidesListingData(t *testing.T) {
	store := testStore(t)
	mustPut(t, store, "appleCertificate", "TEAM/DISTRIBUTION", map[string]string{"private": "secret"})
	if _, err := os.Stat(filepath.Join(store.Directory, "appleCertificate__TEAM%2FDISTRIBUTION.json")); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	store.Register(mux)
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest("GET", "/secrets", nil))
	if list.Code != 200 || strings.Contains(list.Body.String(), "private") || strings.Contains(list.Body.String(), `"data"`) {
		t.Fatalf("listing leaked data: %s", list.Body.String())
	}
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest("GET", "/secrets/appleCertificate/TEAM%2FDISTRIBUTION", nil))
	if get.Code != 200 || !strings.Contains(get.Body.String(), "secret") {
		t.Fatalf("encoded name failed: %d %s", get.Code, get.Body.String())
	}
	original, _ := store.Get("appleCertificate", "TEAM/DISTRIBUTION", "")
	updated, err := store.Put("appleCertificate", "TEAM/DISTRIBUTION", map[string]string{"private": "replacement"}, "")
	if err != nil || updated.CreatedAt != original.CreatedAt {
		t.Fatalf("creation time changed: %v", err)
	}
}
func TestSigningProfileMatchesCertificateBundleAndDevice(t *testing.T) {
	store := testStore(t)
	request := Request{TeamID: "TEAM", BundleID: "com.example.app", DeviceUDID: "AB-CD"}
	cert := map[string]string{"serialNumber": "SERIAL", "expirationDate": "2000-01-01T00:00:00Z"}
	mustPut(t, store, "appleCertificate", "TEAM/DISTRIBUTION", cert)
	if _, _, err := store.SigningCredentials(request, false); err == nil {
		t.Fatal("accepted expired certificate")
	}
	cert["expirationDate"] = "2100-01-01T00:00:00Z"
	mustPut(t, store, "appleCertificate", "TEAM/DISTRIBUTION", cert)
	profile := map[string]string{
		"teamID":                   "TEAM",
		"bundleIDs":                " com.example.app ",
		"certificateSerialNumbers": "OTHER",
		"expirationDate":           "2100-01-01T00:00:00Z",
	}
	mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", profile)
	if _, _, err := store.SigningCredentials(request, false); err == nil {
		t.Fatal("accepted profile for another certificate")
	}
	profile["certificateSerialNumbers"] = " SERIAL "
	mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", profile)
	if _, _, err := store.SigningCredentials(request, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.SigningCredentials(request, true); err == nil {
		t.Fatal("accepted App Store profile for a device build")
	}
	profile["deviceIDs"] = `[{"udid":"abcd"}]`
	mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", profile)
	if _, _, err := store.SigningCredentials(request, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.SigningCredentials(request, false); err == nil {
		t.Fatal("accepted device profile for store publishing")
	}
	request.DeviceUDID = "FFFF"
	if _, _, err := store.SigningCredentials(request, true); err == nil {
		t.Fatal("accepted uncovered device")
	}
}
func TestWebhookAuthenticationAndCorrelation(t *testing.T) {
	jobs := NewJobs()
	defer jobs.Close()
	id, token, err := jobs.begin(Request{}, "publish")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := jobs.begin(Request{}, "publish")
	if err != nil {
		t.Fatal(err)
	}
	handler := jobs.WebhookHandler("X-Publish-Token")
	for _, credential := range []string{"", "wrong", token} {
		r := httptest.NewRequest("POST", "/arbitrary/callback/path", strings.NewReader(`{"status":"SUCCEEDED"}`))
		r.Header.Set("X-Publish-Token", credential)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		want := 404
		if credential == token {
			want = 204
		}
		if response.Code != want {
			t.Fatalf("callback status %d, want %d", response.Code, want)
		}
	}
	status, _ := jobs.Get(id)
	unrelated, _ := jobs.Get(other)
	if status.State != "succeeded" || unrelated.State != "running" {
		t.Fatalf("bad correlation: %+v %+v", status, unrelated)
	}
	jobs.fail(id, "late CLI error")
	status, _ = jobs.Get(id)
	if status.State != "succeeded" {
		t.Fatal("late CLI failure overwrote a completed webhook")
	}
}
func TestDetachedCLIWorkflows(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	name := "lim"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	input, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(filepath.Join(bin, name), os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(output, input); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, kind := range []string{"cloud", "manual", "android", "install"} {
		t.Run(kind, func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "cli.json")
			t.Setenv("EXAMPLE_CLI_CAPTURE", capture)
			store := testStore(t)
			mustPut(t, store, "appStoreConnectApiKey", "TEAM/APP_STORE_CONNECT_API_KEY", map[string]string{"keyId": "KEY", "issuerId": "ISSUER", "privateKeyP8Base64": "a2V5"})
			mustPut(t, store, "appleCertificate", "TEAM/DISTRIBUTION", map[string]string{
				"serialNumber":         "SERIAL",
				"certificateP12Base64": "Y2VydA==",
				"certificatePassword":  "certificate-password",
			})
			profile := map[string]string{
				"teamID":                    "TEAM",
				"bundleIDs":                 "com.example.app",
				"certificateSerialNumbers":  "SERIAL",
				"provisioningProfileBase64": "cHJvZmlsZQ==",
			}
			if kind == "install" {
				profile["deviceIDs"] = `[{"udid":"abcd"}]`
			}
			mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", profile)
			mustPut(t, store, "androidSigningKey", "com.example.app/UPLOAD", map[string]string{
				"keystoreBase64":   "a2V5",
				"keystorePassword": "store-password",
				"keyAlias":         "upload",
				"keyPassword":      "key-password",
			})
			request := Request{
				ProjectPath:       t.TempDir(),
				TeamID:            "TEAM",
				BundleID:          "com.example.app",
				SigningMode:       kind,
				DeviceUDID:        "AB-CD",
				PackageName:       "com.example.app",
				GoogleAccessToken: "play-token",
				WebhookURL:        "https://hooks.example/callback",
			}
			operation := kind
			if kind == "cloud" || kind == "manual" {
				operation = "publish"
			}
			jobs := NewJobs()
			defer jobs.Close()
			id, err := jobs.Start(context.Background(), store, operation, request)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Args             []string `json:"args"`
				KeystorePassword string   `json:"keystorePassword"`
				KeyPassword      string   `json:"keyPassword"`
				PlayToken        string   `json:"playToken"`
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				data, err := os.ReadFile(capture)
				if err == nil && json.Unmarshal(data, &got) == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("fake CLI did not run")
				}
				time.Sleep(10 * time.Millisecond)
			}
			value := func(flag string) string {
				i := slices.Index(got.Args, flag)
				if i < 0 || i+1 >= len(got.Args) {
					return ""
				}
				return got.Args[i+1]
			}
			if value("--webhook-url") != request.WebhookURL || !slices.Contains(got.Args, "--detach") {
				t.Fatalf("bad detached build arguments: %v", got.Args)
			}
			if kind == "cloud" && (value("--signing-method") != "app-store-connect" || slices.Contains(got.Args, "--certificate-p12")) {
				t.Fatal("cloud signing arguments differ")
			}
			if kind == "manual" && (value("--certificate-p12") == "" || value("--provisioning-profile") == "" || slices.Contains(got.Args, "--signing-method")) {
				t.Fatal("manual signing arguments differ")
			}
			if kind == "cloud" || kind == "manual" {
				if !slices.Contains(got.Args, "--upload-to-appstore") {
					t.Fatal("missing App Store upload")
				}
			}
			if kind == "android" {
				if value("--playstore-track") != "internal" || got.KeystorePassword != "store-password" || got.KeyPassword != "key-password" || got.PlayToken != "play-token" {
					t.Fatal("Android environment/track differ")
				}
				for _, secret := range []string{"store-password", "key-password", "play-token"} {
					if slices.Contains(got.Args, secret) {
						t.Fatal("Android secret in arguments")
					}
				}
			}
			status, _ := jobs.Get(id)
			if kind == "install" && value("--upload") != status.AssetName {
				t.Fatal("install asset does not match status")
			}
			header := value("--webhook-header")
			_, token, _ := strings.Cut(header, "=")
			if jobs.Receive(token, json.RawMessage(`{"status":"SUCCEEDED"}`)) != id {
				t.Fatal("CLI webhook token does not authorize its job")
			}
			// Let the CLI exit and verify temporary credential files are removed.
			credentialPath := value("--asc-key")
			if kind == "install" {
				credentialPath = value("--certificate-p12")
			}
			if kind == "android" {
				credentialPath = value("--keystore")
			}
			for {
				_, err := os.Stat(filepath.Dir(credentialPath))
				if os.IsNotExist(err) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("temporary credentials were not removed")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}
func TestExpoIdentifiersPreserveExistingValuesAndFindNestedProjects(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "apps", "mobile")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "app.json")
	if err := os.WriteFile(file, []byte(`{"expo":{"name":"Demo","ios":{"bundleIdentifier":"com.original.app"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureExpoIdentifier(root, "ios", "bundleIdentifier", "com.changed.app"); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureExpoIdentifier(root, "android", "package", "com.android.app"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "com.original.app") || strings.Contains(string(data), "com.changed.app") {
		t.Fatal("replaced existing bundle ID")
	}
	if name := DetectAndroidPackage(root); name != "com.android.app" {
		t.Fatalf("package %q", name)
	}
}
