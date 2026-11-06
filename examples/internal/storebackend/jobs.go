package storebackend

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
)

type Status struct {
	ID                string          `json:"id"`
	State             string          `json:"state"`
	StartedAt         string          `json:"startedAt"`
	ConsoleURL        string          `json:"consoleUrl,omitempty"`
	DeviceUDID        string          `json:"deviceUDID,omitempty"`
	AssetName         string          `json:"assetName,omitempty"`
	Webhook           json.RawMessage `json:"webhook,omitempty"`
	WebhookReceivedAt string          `json:"webhookReceivedAt,omitempty"`
	Error             string          `json:"error,omitempty"`
}
type entry struct {
	status Status
	token  string
	timer  *time.Timer
}
type Jobs struct {
	mu      sync.Mutex
	entries map[string]*entry
}

func NewJobs() *Jobs { return &Jobs{entries: map[string]*entry{}} }
func (j *Jobs) begin(request Request, kind string) (string, string, error) {
	random := make([]byte, 48)
	if _, err := rand.Read(random); err != nil {
		return "", "", err
	}
	// UUIDv4 ID plus an independent 256-bit webhook secret.
	random[6] = (random[6] & 0x0f) | 0x40
	random[8] = (random[8] & 0x3f) | 0x80
	id := fmt.Sprintf("%x-%x-%x-%x-%x", random[:4], random[4:6], random[6:8], random[8:10], random[10:16])
	token := hex.EncodeToString(random[16:])
	e := &entry{
		token:  token,
		status: Status{ID: id, State: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	}
	if kind == "install" {
		e.status.DeviceUDID = request.DeviceUDID
		e.status.AssetName = "device-install-" + request.BundleID + "-" + id + ".ipa"
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.entries[id] = e
	e.timer = time.AfterFunc(2*time.Hour, func() { j.fail(id, "No build-finish webhook arrived within two hours.") })
	return id, token, nil
}
func (j *Jobs) Close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, e := range j.entries {
		e.timer.Stop()
	}
}
func (j *Jobs) Get(id string) (Status, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[id]
	if !ok {
		return Status{}, false
	}
	return e.status, true
}
func (j *Jobs) fail(id, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if e := j.entries[id]; e != nil && e.status.State == "running" {
		e.status.State = "failed"
		e.status.Error = message
		e.timer.Stop()
	}
}
func (j *Jobs) Receive(token string, payload json.RawMessage) string {
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
func (j *Jobs) WebhookHandler(header string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		var payload json.RawMessage
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
			exampleutil.Error(w, 400, err)
			return
		}
		id := j.Receive(r.Header.Get(header), payload)
		if id == "" {
			exampleutil.Error(w, 404, fmt.Errorf("build not found"))
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
func (j *Jobs) spawn(ctx context.Context, id string, args []string, workDir string, env map[string]string, parseSummary bool) {
	go func() {
		defer os.RemoveAll(workDir)
		cmd := exec.CommandContext(ctx, "lim", args...)
		cmd.Env = exampleutil.Env(env)
		var stdout bytes.Buffer
		cmd.Stdout = io.MultiWriter(&stdout, jobLog{id})
		cmd.Stderr = jobLog{id}
		if err := cmd.Run(); err != nil {
			j.fail(id, fmt.Sprintf("lim failed: %v; check that it is on PATH and read the backend logs", err))
			return
		}
		if parseSummary {
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
		}
	}()
}

// Only the webhook listener is exposed publicly; the main API stays on loopback.
func Serve(ctx context.Context, api http.Handler, hooks http.Handler) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errors := make(chan error, 2)
	go func() {
		errors <- exampleutil.Serve(ctx, &http.Server{Addr: "127.0.0.1:3000", Handler: exampleutil.CORS(api)})
	}()
	go func() { errors <- exampleutil.Serve(ctx, &http.Server{Addr: ":3001", Handler: hooks}) }()
	log.Print("Local API: http://localhost:3000; webhook receiver: port 3001")
	first := <-errors
	cancel()
	second := <-errors
	if first != nil {
		return first
	}
	return second
}
