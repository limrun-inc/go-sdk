// Builds one device-bound IPA, uploads it as a private Limrun asset, and
// tracks the detached build through its authenticated build-finish webhook.
package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type installRequest struct {
	ProjectPath string `json:"projectPath"`
	TeamID      string `json:"teamId"`
	BundleID    string `json:"bundleId"`
	DeviceUDID  string `json:"deviceUDID"`
	Scheme      string `json:"scheme"`
	// Store directory the signing secrets live in; the backend default when empty.
	SecretsDir string `json:"secretsDir"`
	// Public URL limbuild POSTs the build-finish webhook to, verbatim. It must
	// forward to this backend's webhook receiver for the result to be observed.
	WebhookURL string `json:"webhookUrl"`
}

func (r installRequest) validate() error {
	if r.ProjectPath == "" || r.WebhookURL == "" {
		return fmt.Errorf("projectPath and webhookUrl are required")
	}
	if r.TeamID == "" || r.BundleID == "" {
		return fmt.Errorf("teamId and bundleId are required")
	}
	if r.DeviceUDID == "" {
		return fmt.Errorf("deviceUDID is required")
	}
	return nil
}

type installStatus struct {
	ID         string `json:"id"`
	State      string `json:"state"`
	StartedAt  string `json:"startedAt"`
	DeviceUDID string `json:"deviceUDID,omitempty"`
	AssetName  string `json:"assetName,omitempty"`
	// The build-finish webhook payload, verbatim as limbuild POSTed it.
	Webhook           json.RawMessage `json:"webhook,omitempty"`
	WebhookReceivedAt string          `json:"webhookReceivedAt,omitempty"`
	Error             string          `json:"error,omitempty"`
}

type installEntry struct {
	status installStatus
	// Shared secret limbuild echoes back in the X-Install-Token header.
	token string
	timer *time.Timer
}

// The in-memory install registry: one entry per POST /install for the life of
// the process, which is all a demo needs.
type installs struct {
	mu      sync.Mutex
	entries map[string]*installEntry
}

func newInstalls() *installs { return &installs{entries: map[string]*installEntry{}} }

func (j *installs) begin(request installRequest) (string, string, error) {
	random := make([]byte, 48)
	if _, err := rand.Read(random); err != nil {
		return "", "", err
	}
	// UUIDv4 ID plus an independent 256-bit webhook secret.
	random[6] = (random[6] & 0x0f) | 0x40
	random[8] = (random[8] & 0x3f) | 0x80
	id := fmt.Sprintf("%x-%x-%x-%x-%x", random[:4], random[4:6], random[6:8], random[8:10], random[10:16])
	token := hex.EncodeToString(random[16:])
	e := &installEntry{
		token: token,
		status: installStatus{
			ID:         id,
			State:      "running",
			StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			DeviceUDID: request.DeviceUDID,
			AssetName:  "device-install-" + request.BundleID + "-" + id + ".ipa",
		},
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.entries[id] = e
	// The CLI exits once limbuild accepts the build, so only the webhook says
	// when it finished. The timer keeps a lost callback from running forever.
	e.timer = time.AfterFunc(2*time.Hour, func() { j.fail(id, "No build-finish webhook arrived within two hours.") })
	return id, token, nil
}

func (j *installs) close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, e := range j.entries {
		e.timer.Stop()
	}
}

func (j *installs) get(id string) (installStatus, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[id]
	if !ok {
		return installStatus{}, false
	}
	return e.status, true
}

// fail marks the install failed unless a webhook already settled it.
func (j *installs) fail(id, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if e := j.entries[id]; e != nil && e.status.State == "running" {
		e.status.State = "failed"
		e.status.Error = message
		e.timer.Stop()
	}
}

// receive records the webhook for the install whose token matches. limbuild
// POSTs to the webhook URL exactly as provided, so the per-install token is
// both the authentication and the correlation. It returns the settled ID.
func (j *installs) receive(token string, payload json.RawMessage) string {
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
func (j *installs) webhookHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		var payload json.RawMessage
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
			writeError(w, 400, err)
			return
		}
		id := j.receive(r.Header.Get("X-Install-Token"), payload)
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
// log, and deletes the materialized credentials once it exits.
func (j *installs) spawn(ctx context.Context, id string, args []string, workDir string) {
	go func() {
		defer os.RemoveAll(workDir)
		cmd := exec.CommandContext(ctx, "lim", args...)
		cmd.Stdout = jobLog{id}
		cmd.Stderr = jobLog{id}
		if err := cmd.Run(); err != nil {
			j.fail(id, fmt.Sprintf("lim failed: %v; check that it is on PATH and read the backend logs", err))
		}
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

// start materializes the ad-hoc signing credentials and spawns a detached
// `lim xcode build`. The outcome arrives through the webhook, not the CLI output.
func (j *installs) start(ctx context.Context, store *secretStore, request installRequest) (string, error) {
	certificate, profile, err := store.signingCredentials(request)
	if err != nil {
		return "", err
	}
	workDir, err := os.MkdirTemp("", "limrun-install-")
	if err != nil {
		return "", err
	}
	spawned := false
	defer func() {
		if !spawned {
			os.RemoveAll(workDir)
		}
	}()
	certificatePath, err := materialize(workDir, "certificate.p12", certificate.Data["certificateP12Base64"])
	if err != nil {
		return "", err
	}
	profilePath, err := materialize(workDir, "profile.mobileprovision", profile.Data["provisioningProfileBase64"])
	if err != nil {
		return "", err
	}
	args := []string{
		"xcode", "build", request.ProjectPath, "--sdk", "iphoneos", "--configuration", "Release",
		"--certificate-p12", certificatePath,
		"--certificate-password", certificate.Data["certificatePassword"],
		"--provisioning-profile", profilePath,
	}
	if request.Scheme != "" {
		args = append(args, "--scheme", request.Scheme)
	}
	updates, err := ensureExpoBundleIdentifier(request.ProjectPath, request.BundleID)
	if err != nil {
		return "", err
	}
	id, token, err := j.begin(request)
	if err != nil {
		return "", err
	}
	status, _ := j.get(id)
	args = append(args, "--upload", status.AssetName, "--webhook-url", request.WebhookURL, "--webhook-header", "X-Install-Token="+token, "--inactivity-timeout", "3s", "--detach")
	for _, line := range updates {
		log.Printf("[%s] %s", id, line)
	}
	log.Printf("[%s] starting detached install build", id)
	j.spawn(ctx, id, args, workDir)
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

// ensureExpoBundleIdentifier fills a missing expo.ios.bundleIdentifier, which
// prebuild needs, and leaves an existing value alone. It returns log lines.
func ensureExpoBundleIdentifier(root, bundleID string) ([]string, error) {
	files := findExpoConfigs(root, 4)
	if len(files) == 0 {
		return nil, nil
	}
	if len(files) > 1 {
		return []string{fmt.Sprintf("Multiple Expo app.json files: set expo.ios.bundleIdentifier to %s manually.", bundleID)}, nil
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
	ios, _ := expo["ios"].(map[string]any)
	if ios == nil {
		ios = map[string]any{}
		expo["ios"] = ios
	}
	if existing, ok := ios["bundleIdentifier"].(string); ok && existing != "" {
		if existing != bundleID {
			return []string{
				fmt.Sprintf("%s declares %s, but the build targets %s; leaving the existing value unchanged.", files[0], existing, bundleID),
			}, nil
		}
		return nil, nil
	}
	ios["bundleIdentifier"] = bundleID
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(files[0], append(encoded, '\n'), 0644); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("Set expo.ios.bundleIdentifier in %s to %s.", files[0], bundleID)}, nil
}
