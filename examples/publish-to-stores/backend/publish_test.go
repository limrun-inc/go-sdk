package main

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

// The test binary stands in for lim when EXAMPLE_CLI_CAPTURE names a file: it
// records its arguments and secret environment there, prints a detach summary
// and exits, so no cloud build or store publish runs.
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

// installFakeCLI puts a copy of the test binary named lim first on PATH and
// returns the file it records its invocation in.
func installFakeCLI(t *testing.T) string {
	t.Helper()
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
	capture := filepath.Join(bin, "cli.json")
	t.Setenv("EXAMPLE_CLI_CAPTURE", capture)
	return capture
}

type cliInvocation struct {
	Args             []string `json:"args"`
	KeystorePassword string   `json:"keystorePassword"`
	KeyPassword      string   `json:"keyPassword"`
	PlayToken        string   `json:"playToken"`
}

// capturedInvocation waits for the fake CLI to run and returns what it recorded.
func capturedInvocation(t *testing.T, capture string) cliInvocation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got cliInvocation
		data, err := os.ReadFile(capture)
		if err == nil && json.Unmarshal(data, &got) == nil {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatal("fake CLI did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func argValue(args []string, flag string) string {
	i := slices.Index(args, flag)
	if i < 0 || i+1 >= len(args) {
		return ""
	}
	return args[i+1]
}

func mustPut(t *testing.T, s *secretStore, kind, name string, data map[string]string) {
	t.Helper()
	if _, err := s.put(kind, name, data, ""); err != nil {
		t.Fatal(err)
	}
}

func testStore(t *testing.T) *secretStore {
	t.Helper()
	t.Setenv("SECRETS_DIR", "")
	s, err := newSecretStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSecretStoreIsCompatibleWithEncodedNamesAndHidesListingData(t *testing.T) {
	store := testStore(t)
	mustPut(t, store, "appleCertificate", "TEAM/DISTRIBUTION", map[string]string{"private": "secret"})
	if _, err := os.Stat(filepath.Join(store.directory, "appleCertificate__TEAM%2FDISTRIBUTION.json")); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	store.register(mux)
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
	original, _ := store.get("appleCertificate", "TEAM/DISTRIBUTION", "")
	updated, err := store.put("appleCertificate", "TEAM/DISTRIBUTION", map[string]string{"private": "replacement"}, "")
	if err != nil || updated.CreatedAt != original.CreatedAt {
		t.Fatalf("creation time changed: %v", err)
	}
}

func TestSigningProfileMatchesCertificateAndBundle(t *testing.T) {
	store := testStore(t)
	request := publishRequest{TeamID: "TEAM", BundleID: "com.example.app"}
	cert := map[string]string{"serialNumber": "SERIAL", "expirationDate": "2000-01-01T00:00:00Z"}
	mustPut(t, store, "appleCertificate", "TEAM/DISTRIBUTION", cert)
	if _, _, err := store.signingCredentials(request); err == nil {
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
	if _, _, err := store.signingCredentials(request); err == nil {
		t.Fatal("accepted profile for another certificate")
	}
	profile["certificateSerialNumbers"] = " SERIAL "
	mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", profile)
	if _, _, err := store.signingCredentials(request); err != nil {
		t.Fatal(err)
	}
	profile["deviceIDs"] = `[{"udid":"abcd"}]`
	mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", profile)
	if _, _, err := store.signingCredentials(request); err == nil {
		t.Fatal("accepted device profile for store publishing")
	}
}

func TestWebhookAuthenticationAndCorrelation(t *testing.T) {
	jobs := newPublishes()
	defer jobs.close()
	id, token, err := jobs.begin()
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := jobs.begin()
	if err != nil {
		t.Fatal(err)
	}
	handler := jobs.webhookHandler()
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
	status, _ := jobs.get(id)
	unrelated, _ := jobs.get(other)
	if status.State != "succeeded" || unrelated.State != "running" {
		t.Fatalf("bad correlation: %+v %+v", status, unrelated)
	}
	jobs.fail(id, "late CLI error")
	status, _ = jobs.get(id)
	if status.State != "succeeded" {
		t.Fatal("late CLI failure overwrote a completed webhook")
	}
}

func TestDetachedCLIWorkflows(t *testing.T) {
	for _, kind := range []string{"cloud", "manual", "android"} {
		t.Run(kind, func(t *testing.T) {
			capture := installFakeCLI(t)
			store := testStore(t)
			mustPut(t, store, "appStoreConnectApiKey", "TEAM/APP_STORE_CONNECT_API_KEY", map[string]string{"keyId": "KEY", "issuerId": "ISSUER", "privateKeyP8Base64": "a2V5"})
			mustPut(t, store, "appleCertificate", "TEAM/DISTRIBUTION", map[string]string{
				"serialNumber":         "SERIAL",
				"certificateP12Base64": "Y2VydA==",
				"certificatePassword":  "certificate-password",
			})
			mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", map[string]string{
				"teamID":                    "TEAM",
				"bundleIDs":                 "com.example.app",
				"certificateSerialNumbers":  "SERIAL",
				"provisioningProfileBase64": "cHJvZmlsZQ==",
			})
			mustPut(t, store, "androidSigningKey", "com.example.app/UPLOAD", map[string]string{
				"keystoreBase64":   "a2V5",
				"keystorePassword": "store-password",
				"keyAlias":         "upload",
				"keyPassword":      "key-password",
			})
			webhookURL := "https://hooks.example/callback"
			jobs := newPublishes()
			defer jobs.close()
			var id string
			var err error
			if kind == "android" {
				id, err = jobs.startAndroid(context.Background(), store, androidPublishRequest{
					ProjectPath:       t.TempDir(),
					PackageName:       "com.example.app",
					GoogleAccessToken: "play-token",
					WebhookURL:        webhookURL,
				})
			} else {
				id, err = jobs.start(context.Background(), store, publishRequest{
					ProjectPath: t.TempDir(),
					TeamID:      "TEAM",
					BundleID:    "com.example.app",
					SigningMode: kind,
					WebhookURL:  webhookURL,
				})
			}
			if err != nil {
				t.Fatal(err)
			}
			got := capturedInvocation(t, capture)
			if argValue(got.Args, "--webhook-url") != webhookURL || !slices.Contains(got.Args, "--detach") {
				t.Fatalf("bad detached build arguments: %v", got.Args)
			}
			if kind == "cloud" && (argValue(got.Args, "--signing-method") != "app-store-connect" || slices.Contains(got.Args, "--certificate-p12")) {
				t.Fatal("cloud signing arguments differ")
			}
			if kind == "manual" && (argValue(got.Args, "--certificate-p12") == "" || argValue(got.Args, "--provisioning-profile") == "" || slices.Contains(got.Args, "--signing-method")) {
				t.Fatal("manual signing arguments differ")
			}
			if kind != "android" && !slices.Contains(got.Args, "--upload-to-appstore") {
				t.Fatal("missing App Store upload")
			}
			if kind == "android" {
				if argValue(got.Args, "--playstore-track") != "internal" || got.KeystorePassword != "store-password" || got.KeyPassword != "key-password" || got.PlayToken != "play-token" {
					t.Fatal("Android environment/track differ")
				}
				for _, secret := range []string{"store-password", "key-password", "play-token"} {
					if slices.Contains(got.Args, secret) {
						t.Fatal("Android secret in arguments")
					}
				}
			}
			_, token, _ := strings.Cut(argValue(got.Args, "--webhook-header"), "=")
			if jobs.receive(token, json.RawMessage(`{"status":"SUCCEEDED"}`)) != id {
				t.Fatal("CLI webhook token does not authorize its job")
			}
			// Let the CLI exit and verify temporary credential files are removed.
			credentialPath := argValue(got.Args, "--asc-key")
			if kind == "android" {
				credentialPath = argValue(got.Args, "--keystore")
			}
			deadline := time.Now().Add(5 * time.Second)
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
			status, _ := jobs.get(id)
			if status.ConsoleURL != "https://console.example/build" {
				t.Fatalf("console link not taken from the detach summary: %+v", status)
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
	if _, err := ensureExpoIdentifier(root, "ios", "bundleIdentifier", "com.changed.app"); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureExpoIdentifier(root, "android", "package", "com.android.app"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "com.original.app") || strings.Contains(string(data), "com.changed.app") {
		t.Fatal("replaced existing bundle ID")
	}
	if name := detectAndroidPackage(root); name != "com.android.app" {
		t.Fatalf("package %q", name)
	}
	// Like the TypeScript isFile check, discovery must not follow a symlinked
	// app.json out of the project root.
	outside := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(outside, []byte(`{"expo":{"android":{"package":"com.outside.app"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "app.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if configs := findExpoConfigs(root, 4); len(configs) != 1 || configs[0] != file {
		t.Fatalf("discovery followed a symlink: %v", configs)
	}
}
