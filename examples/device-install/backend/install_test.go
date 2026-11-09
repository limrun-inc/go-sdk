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
// records its arguments there and exits, so no cloud build runs.
func TestMain(m *testing.M) {
	if capture := os.Getenv("EXAMPLE_CLI_CAPTURE"); capture != "" {
		data, _ := json.Marshal(os.Args[1:])
		if err := os.WriteFile(capture, data, 0600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// installFakeCLI puts a copy of the test binary named lim first on PATH and
// returns the file it records its arguments in.
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

// capturedArgs waits for the fake CLI to run and returns its arguments.
func capturedArgs(t *testing.T, capture string) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var args []string
		data, err := os.ReadFile(capture)
		if err == nil && json.Unmarshal(data, &args) == nil {
			return args
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

func TestSigningProfileMatchesCertificateBundleAndDevice(t *testing.T) {
	store := testStore(t)
	request := installRequest{TeamID: "TEAM", BundleID: "com.example.app", DeviceUDID: "AB-CD"}
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
		"deviceIDs":                `[{"udid":"abcd"}]`,
	}
	mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", profile)
	if _, _, err := store.signingCredentials(request); err == nil {
		t.Fatal("accepted profile for another certificate")
	}
	profile["certificateSerialNumbers"] = " SERIAL "
	delete(profile, "deviceIDs")
	mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", profile)
	if _, _, err := store.signingCredentials(request); err == nil {
		t.Fatal("accepted App Store profile for a device build")
	}
	profile["deviceIDs"] = `[{"udid":"abcd"}]`
	mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", profile)
	if _, _, err := store.signingCredentials(request); err != nil {
		t.Fatal(err)
	}
	request.DeviceUDID = "FFFF"
	if _, _, err := store.signingCredentials(request); err == nil {
		t.Fatal("accepted uncovered device")
	}
}

func TestDetachedInstallWorkflow(t *testing.T) {
	capture := installFakeCLI(t)
	store := testStore(t)
	mustPut(t, store, "appleCertificate", "TEAM/DISTRIBUTION", map[string]string{
		"serialNumber":         "SERIAL",
		"certificateP12Base64": "Y2VydA==",
		"certificatePassword":  "certificate-password",
	})
	mustPut(t, store, "appleProvisioningProfile", "TEAM/PROFILE", map[string]string{
		"teamID":                    "TEAM",
		"bundleIDs":                 "com.example.app",
		"certificateSerialNumbers":  "SERIAL",
		"deviceIDs":                 `[{"udid":"abcd"}]`,
		"provisioningProfileBase64": "cHJvZmlsZQ==",
	})
	request := installRequest{
		ProjectPath: t.TempDir(),
		TeamID:      "TEAM",
		BundleID:    "com.example.app",
		DeviceUDID:  "AB-CD",
		WebhookURL:  "https://hooks.example/callback",
	}
	jobs := newInstalls()
	defer jobs.close()
	id, err := jobs.start(context.Background(), store, request)
	if err != nil {
		t.Fatal(err)
	}
	args := capturedArgs(t, capture)
	if argValue(args, "--webhook-url") != request.WebhookURL || !slices.Contains(args, "--detach") {
		t.Fatalf("bad detached build arguments: %v", args)
	}
	if argValue(args, "--certificate-p12") == "" || argValue(args, "--provisioning-profile") == "" || slices.Contains(args, "--signing-method") {
		t.Fatalf("ad-hoc signing arguments differ: %v", args)
	}
	status, _ := jobs.get(id)
	if argValue(args, "--upload") != status.AssetName {
		t.Fatal("install asset does not match status")
	}
	_, token, _ := strings.Cut(argValue(args, "--webhook-header"), "=")
	if jobs.receive(token, json.RawMessage(`{"status":"SUCCEEDED"}`)) != id {
		t.Fatal("CLI webhook token does not authorize its job")
	}
	// Let the CLI exit and verify temporary credential files are removed.
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := os.Stat(filepath.Dir(argValue(args, "--certificate-p12")))
		if os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("temporary credentials were not removed")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Like the TypeScript isFile check, discovery must not follow a symlinked
// app.json out of the project root.
func TestFindExpoConfigsIgnoresSymlinks(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "apps", "mobile")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "app.json")
	if err := os.WriteFile(file, []byte(`{"expo":{"name":"Demo"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(outside, []byte(`{"expo":{"ios":{"bundleIdentifier":"com.outside.app"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "app.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if configs := findExpoConfigs(root, 4); len(configs) != 1 || configs[0] != file {
		t.Fatalf("discovery followed a symlink: %v", configs)
	}
}
