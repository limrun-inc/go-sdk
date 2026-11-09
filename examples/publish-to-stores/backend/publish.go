// Runs one App Store publish: materializes either the cloud-signing API key
// or manual signing credentials, starts a detached `lim xcode build` with a
// build-finish webhook, and tracks the build until the callback arrives. The
// job registry here also serves the Android flow in android_publish.go.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type publishRequest struct {
	ProjectPath string `json:"projectPath"`
	TeamID      string `json:"teamId"`
	BundleID    string `json:"bundleId"`
	// cloud exports with Apple's cloud-managed certificate; manual passes the
	// stored certificate and App Store profile to the build.
	SigningMode string `json:"signingMode"`
	Scheme      string `json:"scheme"`
	// Store directory the signing secrets live in; the backend default when empty.
	SecretsDir string `json:"secretsDir"`
	// Public URL limbuild POSTs the build-finish webhook to, verbatim. It must
	// forward to this backend's webhook receiver for the result to be observed.
	WebhookURL string `json:"webhookUrl"`
}

func (r publishRequest) validate() error {
	if r.ProjectPath == "" || r.WebhookURL == "" {
		return fmt.Errorf("projectPath and webhookUrl are required")
	}
	if r.TeamID == "" || r.BundleID == "" {
		return fmt.Errorf("teamId and bundleId are required")
	}
	if r.SigningMode != "cloud" && r.SigningMode != "manual" {
		return fmt.Errorf("signingMode must be cloud or manual")
	}
	return nil
}

type publishStatus struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	StartedAt string `json:"startedAt"`
	// Console page of the instance the build runs on, from the CLI's --json
	// detach summary. Known long before the webhook arrives, so the UI can
	// link to live progress.
	ConsoleURL string `json:"consoleUrl,omitempty"`
	// The build-finish webhook payload, verbatim as limbuild POSTed it.
	Webhook           json.RawMessage `json:"webhook,omitempty"`
	WebhookReceivedAt string          `json:"webhookReceivedAt,omitempty"`
	Error             string          `json:"error,omitempty"`
}

type publishEntry struct {
	status publishStatus
	// Shared secret limbuild echoes back in the X-Publish-Token header.
	token string
	timer *time.Timer
}

// The in-memory publish registry: one entry per publish for the life of the
// process, which is all a demo needs.
type publishes struct {
	mu      sync.Mutex
	entries map[string]*publishEntry
}

func newPublishes() *publishes { return &publishes{entries: map[string]*publishEntry{}} }

// begin allocates the authenticated callback state shared by iOS and Android publishes.
func (j *publishes) begin() (string, string, error) {
	random := make([]byte, 48)
	if _, err := rand.Read(random); err != nil {
		return "", "", err
	}
	// UUIDv4 ID plus an independent 256-bit webhook secret.
	random[6] = (random[6] & 0x0f) | 0x40
	random[8] = (random[8] & 0x3f) | 0x80
	id := fmt.Sprintf("%x-%x-%x-%x-%x", random[:4], random[4:6], random[6:8], random[8:10], random[10:16])
	token := hex.EncodeToString(random[16:])
	e := &publishEntry{
		token:  token,
		status: publishStatus{ID: id, State: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.entries[id] = e
	// The CLI exits once limbuild accepts the build, so only the webhook says
	// when it finished. The timer keeps a lost callback from running forever.
	e.timer = time.AfterFunc(2*time.Hour, func() { j.fail(id, "No build-finish webhook arrived within two hours.") })
	return id, token, nil
}

func (j *publishes) close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, e := range j.entries {
		e.timer.Stop()
	}
}

func (j *publishes) get(id string) (publishStatus, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[id]
	if !ok {
		return publishStatus{}, false
	}
	return e.status, true
}

// fail marks the publish failed unless a webhook already settled it.
func (j *publishes) fail(id, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if e := j.entries[id]; e != nil && e.status.State == "running" {
		e.status.State = "failed"
		e.status.Error = message
		e.timer.Stop()
	}
}

// receive records the webhook for the publish whose token matches. limbuild
// POSTs to the webhook URL exactly as provided, so the per-publish token is
// both the authentication and the correlation. It returns the settled ID.
func (j *publishes) receive(token string, payload json.RawMessage) string {
	j.mu.Lock()
	defer j.mu.Unlock()
	for id, e := range j.entries {
		if subtle.ConstantTimeCompare([]byte(token), []byte(e.token)) != 1 {
			continue
		}
		var body struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		_ = json.Unmarshal(payload, &body)
		e.status.Webhook = append(json.RawMessage(nil), payload...)
		e.status.WebhookReceivedAt = time.Now().UTC().Format(time.RFC3339Nano)
		e.status.State = "failed"
		e.status.Error = body.Error
		if body.Status == "SUCCEEDED" {
			e.status.State = "succeeded"
			e.status.Error = ""
		} else if e.status.Error == "" {
			e.status.Error = fmt.Sprintf("Build finished with status %q.", body.Status)
		}
		e.timer.Stop()
		return id
	}
	return ""
}

// webhookHandler accepts the build-finish POST on any path, since nothing is
// appended to the entered webhook URL.
func (j *publishes) webhookHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		var payload json.RawMessage
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
			writeError(w, 400, err)
			return
		}
		id := j.receive(r.Header.Get("X-Publish-Token"), payload)
		if id == "" {
			writeError(w, 404, fmt.Errorf("build not found"))
			return
		}
		log.Printf("[%s] received build-finish webhook", id)
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

type jobLog struct{ id string }

func (w jobLog) Write(data []byte) (int, error) {
	log.Printf("[%s] %s", w.id, data)
	return len(data), nil
}

// spawn runs the CLI in the background, forwards its output to this process's
// log, records the console link from its detach summary, and deletes the
// materialized credentials once it exits. A nil env inherits this process's.
func (j *publishes) spawn(ctx context.Context, id string, args []string, workDir string, env []string) {
	go func() {
		defer os.RemoveAll(workDir)
		cmd := exec.CommandContext(ctx, "lim", args...)
		cmd.Env = env
		var stdout bytes.Buffer
		cmd.Stdout = io.MultiWriter(&stdout, jobLog{id})
		cmd.Stderr = jobLog{id}
		if err := cmd.Run(); err != nil {
			j.fail(id, fmt.Sprintf("lim failed: %v; check that it is on PATH and read the backend logs", err))
			return
		}
		// The build runs with --json, which suppresses progress output, so on
		// a clean exit stdout is exactly the summary object.
		var summary struct {
			ConsoleURL string `json:"consoleUrl"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
			log.Printf("[%s] could not parse the CLI detach summary", id)
			return
		}
		j.mu.Lock()
		j.entries[id].status.ConsoleURL = summary.ConsoleURL
		j.mu.Unlock()
	}()
}

// materialize writes a base64 secret to a file only the backend can read.
func materialize(dir, name, encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode %s: %w", name, err)
	}
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, data, 0600)
}

// start materializes the selected signing credentials and spawns a detached
// `lim xcode build` that uploads to App Store Connect. The outcome arrives
// through the webhook, not the CLI output.
func (j *publishes) start(ctx context.Context, store *secretStore, request publishRequest) (string, error) {
	key, err := store.get("appStoreConnectApiKey", request.TeamID+"/APP_STORE_CONNECT_API_KEY", request.SecretsDir)
	if err != nil {
		return "", err
	}
	if key == nil {
		return "", fmt.Errorf("no App Store Connect API key; run Connect first")
	}
	if request.SigningMode == "cloud" && key.Data["issuerId"] == "" {
		return "", fmt.Errorf("cloud signing requires a team API key with an issuer ID")
	}
	var certificate, profile *secret
	if request.SigningMode == "manual" {
		certificate, profile, err = store.signingCredentials(request)
		if err != nil {
			return "", err
		}
	}
	workDir, err := os.MkdirTemp("", "limrun-publish-")
	if err != nil {
		return "", err
	}
	spawned := false
	defer func() {
		if !spawned {
			os.RemoveAll(workDir)
		}
	}()
	args := []string{"xcode", "build", request.ProjectPath, "--sdk", "iphoneos", "--configuration", "Release"}
	// Cloud and manual signing are mutually exclusive: either Apple's
	// cloud-managed certificate or the stored certificate and profile.
	if certificate != nil {
		certificatePath, err := materialize(workDir, "certificate.p12", certificate.Data["certificateP12Base64"])
		if err != nil {
			return "", err
		}
		profilePath, err := materialize(workDir, "profile.mobileprovision", profile.Data["provisioningProfileBase64"])
		if err != nil {
			return "", err
		}
		args = append(args, "--certificate-p12", certificatePath, "--certificate-password", certificate.Data["certificatePassword"], "--provisioning-profile", profilePath)
	} else {
		args = append(args, "--signing-method", "app-store-connect", "--team-id", request.TeamID)
	}
	// The App Store Connect API key authorizes the upload in both modes.
	keyPath, err := materialize(workDir, "AuthKey.p8", key.Data["privateKeyP8Base64"])
	if err != nil {
		return "", err
	}
	args = append(args, "--upload-to-appstore", "--auto-build-number", "--asc-key-id", key.Data["keyId"], "--asc-key", keyPath)
	if key.Data["issuerId"] != "" {
		args = append(args, "--asc-issuer-id", key.Data["issuerId"])
	}
	if request.Scheme != "" {
		args = append(args, "--scheme", request.Scheme)
	}
	updates, err := ensureExpoIdentifier(request.ProjectPath, "ios", "bundleIdentifier", request.BundleID)
	if err != nil {
		return "", err
	}
	id, token, err := j.begin()
	if err != nil {
		return "", err
	}
	// --json turns the detach summary into one machine-readable object on stdout.
	args = append(args, "--webhook-url", request.WebhookURL, "--webhook-header", "X-Publish-Token="+token, "--inactivity-timeout", "3s", "--detach", "--json")
	for _, line := range updates {
		log.Printf("[%s] %s", id, line)
	}
	log.Printf("[%s] starting detached publish build", id)
	j.spawn(ctx, id, args, workDir, nil)
	spawned = true
	return id, nil
}

// findExpoConfigs returns the app.json files with an "expo" key under root,
// so monorepos work without pointing the wizard at the app directory itself.
func findExpoConfigs(root string, depth int) []string {
	var found []string
	entries, err := os.ReadDir(root)
	if err != nil {
		return found
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if entry.Type().IsRegular() && entry.Name() == "app.json" {
			if data, err := os.ReadFile(path); err == nil {
				var config map[string]any
				if json.Unmarshal(data, &config) == nil && config["expo"] != nil {
					found = append(found, path)
				}
			}
		} else if entry.IsDir() && depth > 0 {
			found = append(found, findExpoConfigs(path, depth-1)...)
		}
	}
	return found
}

// ensureExpoIdentifier fills a missing expo.<platform>.<key>, which prebuild
// needs for the app identifier, and leaves an existing value alone. It
// returns log lines.
func ensureExpoIdentifier(root, platform, key, value string) ([]string, error) {
	files := findExpoConfigs(root, 4)
	if len(files) == 0 {
		return nil, nil
	}
	if len(files) > 1 {
		return []string{fmt.Sprintf("Multiple Expo app.json files: set expo.%s.%s to %s manually.", platform, key, value)}, nil
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	expo, ok := config["expo"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expo must be an object in %s", files[0])
	}
	section, _ := expo[platform].(map[string]any)
	if section == nil {
		section = map[string]any{}
		expo[platform] = section
	}
	if existing, ok := section[key].(string); ok && existing != "" {
		if existing != value {
			return []string{
				fmt.Sprintf("%s declares %s, but the build targets %s; leaving the existing value unchanged.", files[0], existing, value),
			}, nil
		}
		return nil, nil
	}
	section[key] = value
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(files[0], append(encoded, '\n'), 0644); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("Set expo.%s.%s in %s to %s.", platform, key, files[0], value)}, nil
}
