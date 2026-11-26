package macos_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/build"
	"github.com/limrun-inc/go-sdk/macos"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/session"
)

type recorded struct {
	Method, Path, Auth string
	Body               any
}

// guest stands in for limacvm: HTTP for the routes that stay HTTP, and the
// signaling WebSocket for commands. signals returns each signaling request,
// without its client-chosen id.
func guest(t *testing.T) (srv *httptest.Server, requests func() []recorded, signals func() []string) {
	t.Helper()
	var mu sync.Mutex
	var reqs []recorded
	var sigs []string
	upgrader := websocket.Upgrader{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/signaling" {
			if r.URL.Query().Get("token") != "control" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ws, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer ws.Close()
			for {
				var msg map[string]any
				if err := ws.ReadJSON(&msg); err != nil {
					return
				}
				id := msg["id"]
				delete(msg, "id")
				raw, _ := json.Marshal(msg)
				mu.Lock()
				sigs = append(sigs, string(raw))
				mu.Unlock()
				for _, out := range replies(msg) {
					out["id"] = id
					if err := ws.WriteJSON(out); err != nil {
						return
					}
				}
			}
		}
		raw, _ := io.ReadAll(r.Body)
		var body any
		if len(raw) > 0 && json.Unmarshal(raw, &body) != nil {
			body = string(raw)
		}
		mu.Lock()
		reqs = append(reqs, recorded{r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), body})
		mu.Unlock()
		switch {
		case r.URL.Path == "/exec":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"execId":"run-1"}`))
		case r.URL.Path == "/exec/run-1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("id: 0\nevent: command\ndata: sw_vers\n\n" +
				"id: 1\nevent: stdout\ndata: 27.2.1\n\n" +
				"id: 2\nevent: exitCode\ndata: 3\n\n"))
		case r.URL.Path == "/files":
			_, _ = w.Write([]byte("mp4"))
		case r.URL.Path == "/trusted-cas":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha256":"abc"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	requests = func() []recorded { mu.Lock(); defer mu.Unlock(); return append([]recorded(nil), reqs...) }
	signals = func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), sigs...) }
	return srv, requests, signals
}

// replies are what limacvm answers a signaling request with, in order.
func replies(msg map[string]any) []map[string]any {
	kind, _ := msg["type"].(string)
	result := map[string]any{"type": kind + "Result"}
	switch kind {
	case "screenshot":
		result["base64"] = base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff})
		result["width"], result["height"] = 3, 1
	case "typeText":
		if msg["text"] == "fail" {
			result["error"] = "no session"
		}
	case "enableSsh":
		result["user"], result["port"] = "limrun", 22
	case "restoreSnapshot":
		if msg["key"] == "broken" {
			return []map[string]any{
				{"type": "restoreSnapshotProgress", "progress": map[string]any{"phase": "failed", "reason": "download_failed"}},
				{"type": "restoreSnapshotResult", "error": "download failed"},
			}
		}
		if msg["key"] == nil {
			return []map[string]any{{"type": "restoreSnapshotResult", "progress": map[string]any{"phase": "skipped"}}}
		}
		return []map[string]any{
			{"type": "restoreSnapshotProgress", "progress": map[string]any{"phase": "resolving"}},
			{"type": "restoreSnapshotProgress", "progress": map[string]any{"phase": "downloading", "totalBytes": 10, "doneBytes": 4}},
			{"type": "restoreSnapshotResult", "progress": map[string]any{"phase": "restored", "key": "my-mac", "matchKind": "exact_hit"}},
		}
	case "startVideoRecordingSession":
		result["recordingId"], result["captureId"] = "rec-1", "rec-123"
	case "stopVideoRecordingSession":
		result["recordingId"] = "rec-1"
	case "launchApp":
		result["bundleId"], result["pid"], result["launched"] = "com.example.app", 42, true
	case "appLogTail":
		result["bundleId"], result["logs"] = msg["bundleId"], "one\ntwo"
	case "streamAppLog":
		return []map[string]any{
			{"type": "appLogStream", "bundleId": msg["bundleId"], "lines": []string{"a", "b"}},
			{"type": "appLogStream", "bundleId": msg["bundleId"], "lines": []string{}, "error": "app quit"},
		}
	case "streamAppLogTerminate":
		return nil
	}
	return []map[string]any{result}
}

func connect(t *testing.T, apiURL string, opts ...build.Option) *macos.Client {
	t.Helper()
	mac, err := macos.NewClient(apiURL, "control", opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mac.Close() })
	return mac
}

func TestClientSendsEachRequest(t *testing.T) {
	srv, requests, signals := guest(t)
	ctx := context.Background()
	var stdout, command bytes.Buffer
	mac := connect(t, srv.URL+"/", build.WithExecutionOptions(build.ExecOptions{Stdout: &stdout, Command: &command}))
	if mac.ConnectionState() != "connected" {
		t.Errorf("state = %s", mac.ConnectionState())
	}

	timeout := float64(30)
	proc, err := mac.Run(ctx, "sw_vers", &macos.RunOptions{Cwd: "Desktop", Env: []string{"A=b"}, TimeoutSeconds: &timeout})
	if err != nil {
		t.Fatal(err)
	}
	result, err := proc.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 3 || result.Status != "FAILED" {
		t.Errorf("result = %+v, want exit code 3", result)
	}
	if stdout.String() != "27.2.1" || command.String() != "sw_vers" {
		t.Errorf("stdout = %q, command = %q", stdout.String(), command.String())
	}

	access, err := mac.EnableSSH(ctx, "ssh-ed25519 AAAA test")
	if err != nil {
		t.Fatal(err)
	}
	if *access != (macos.SSHAccess{User: "limrun", Port: 22}) {
		t.Errorf("access = %+v", access)
	}
	jpeg, err := mac.Screenshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(jpeg, []byte{0xff, 0xd8, 0xff}) {
		t.Errorf("screenshot = %x", jpeg)
	}
	var progress []macos.SnapshotProgress
	restored, err := mac.RestoreSnapshot(ctx, macos.RestoreSnapshotOptions{
		Key:         "my-mac",
		RestoreKeys: []string{"my-mac", "base-"},
		OnProgress:  func(p macos.SnapshotProgress) { progress = append(progress, p) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if *restored != (macos.SnapshotProgress{Phase: "restored", Key: "my-mac", MatchKind: "exact_hit"}) {
		t.Errorf("restored = %+v", restored)
	}
	wantProgress := []macos.SnapshotProgress{
		{Phase: "resolving"},
		{Phase: "downloading", TotalBytes: 10, DoneBytes: 4},
		{Phase: "restored", Key: "my-mac", MatchKind: "exact_hit"},
	}
	if !reflect.DeepEqual(progress, wantProgress) {
		t.Errorf("progress = %+v, want %+v", progress, wantProgress)
	}
	// Empty fields stay out of the request.
	if skipped, err := mac.RestoreSnapshot(ctx, macos.RestoreSnapshotOptions{}); err != nil || skipped.Phase != "skipped" {
		t.Fatalf("RestoreSnapshot = %+v, %v", skipped, err)
	}
	duration := 100
	for _, err := range []error{
		mac.Tap(ctx, macos.Point{X: 10, Y: 20}, &macos.TapOptions{Button: macos.ButtonRight}),
		mac.Tap(ctx, macos.Point{X: 7, Y: 8}, nil),
		mac.TypeText(ctx, "hi"),
		mac.PressKey(ctx, "q", macos.ModifierCommand),
		mac.PressKey(ctx, "enter"),
		mac.Scroll(ctx, macos.ScrollOptions{DY: 300, At: &macos.Point{X: 5, Y: 6}}),
		mac.Scroll(ctx, macos.ScrollOptions{DX: -4}),
		mac.Drag(ctx, macos.Point{X: 1, Y: 2}, macos.Point{X: 3, Y: 4}, &macos.DragOptions{DurationMs: &duration}),
		mac.Drag(ctx, macos.Point{X: 1, Y: 2}, macos.Point{X: 3, Y: 4}, nil),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	var http []string
	for _, r := range requests() {
		if r.Auth != "Bearer control" {
			t.Errorf("%s %s: Authorization = %q", r.Method, r.Path, r.Auth)
		}
		http = append(http, r.Method+" "+r.Path)
	}
	if want := []string{"POST /exec", "GET /exec/run-1/events?follow=true"}; !reflect.DeepEqual(http, want) {
		t.Errorf("HTTP requests = %v, want %v", http, want)
	}
	want := []string{
		`{"publicKey":"ssh-ed25519 AAAA test","type":"enableSsh"}`,
		`{"type":"screenshot"}`,
		`{"key":"my-mac","restoreKeys":["my-mac","base-"],"type":"restoreSnapshot"}`,
		`{"type":"restoreSnapshot"}`,
		`{"button":"right","type":"tap","x":10,"y":20}`,
		`{"type":"tap","x":7,"y":8}`,
		`{"text":"hi","type":"typeText"}`,
		`{"key":"q","modifiers":["command"],"type":"pressKey"}`,
		`{"key":"enter","type":"pressKey"}`,
		`{"coordinate":[5,6],"dx":0,"dy":300,"type":"scroll"}`,
		`{"dx":-4,"dy":0,"type":"scroll"}`,
		`{"durationMs":100,"fromX":1,"fromY":2,"toX":3,"toY":4,"type":"drag"}`,
		`{"fromX":1,"fromY":2,"toX":3,"toY":4,"type":"drag"}`,
	}
	if got := signals(); !reflect.DeepEqual(got, want) {
		t.Errorf("signals =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestRunDefaultsCwdToHome(t *testing.T) {
	srv, requests, _ := guest(t)
	proc, err := connect(t, srv.URL).Run(context.Background(), "true", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proc.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	body := requests()[0].Body.(map[string]any)
	if body["cwd"] != "." {
		t.Errorf("cwd = %v, want .", body["cwd"])
	}
}

func TestErrorsCarryTheInstanceMessage(t *testing.T) {
	srv, _, _ := guest(t)
	err := connect(t, srv.URL).TypeText(context.Background(), "fail")
	if err == nil || err.Error() != "no session" {
		t.Errorf("err = %v, want no session", err)
	}
}

func TestNewClientFailsWithoutTheSignalingSocket(t *testing.T) {
	srv, _, _ := guest(t)
	if _, err := macos.NewClient(srv.URL, "wrong"); err == nil {
		t.Error("a refused signaling socket must fail NewClient")
	}
}

func TestRestoreSnapshotReturnsAFailedRestore(t *testing.T) {
	srv, _, _ := guest(t)
	last, err := connect(t, srv.URL).RestoreSnapshot(context.Background(), macos.RestoreSnapshotOptions{Key: "broken"})
	if err != nil || last.Phase != "failed" || last.Reason != "download_failed" {
		t.Fatalf("RestoreSnapshot = %+v, %v", last, err)
	}
}

func TestStreamAppLogEndsWithTheInstanceError(t *testing.T) {
	srv, _, signals := guest(t)
	stream := connect(t, srv.URL).StreamAppLog(context.Background(), "com.apple.TextEdit")
	var lines []string
	for line := range stream.Entries() {
		lines = append(lines, line)
	}
	if err := <-stream.Err(); err == nil || err.Error() != "app quit" {
		t.Errorf("err = %v, want app quit", err)
	}
	if !reflect.DeepEqual(lines, []string{"a", "b"}) {
		t.Errorf("lines = %v", lines)
	}
	if got := signals(); !reflect.DeepEqual(got, []string{`{"bundleId":"com.apple.TextEdit","type":"streamAppLog"}`}) {
		t.Errorf("signals = %v", got)
	}
}

func TestDerivePortForwardURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://mac.example.test/v1/abc/": "wss://mac.example.test/v1/abc/port-forward?port=22",
		"http://127.0.0.1:8080":            "ws://127.0.0.1:8080/port-forward?port=22",
	} {
		got, err := macos.DerivePortForwardURL(in, 22)
		if err != nil || got != want {
			t.Errorf("DerivePortForwardURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := macos.DerivePortForwardURL("ftp://x", 22); err == nil {
		t.Error("ftp URL must fail")
	}
}

// TestStartPortForwardCarriesBytes runs a multiplexed tunnel against a server
// that echoes each connection's payload back, and checks the handshake asks
// for the remote port with the instance token.
func TestStartPortForwardCarriesBytes(t *testing.T) {
	type handshake struct{ path, port, mode, auth string }
	seen := make(chan handshake, 1)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/signaling" {
			// The client's signaling socket: accept it and wait.
			if ws, err := upgrader.Upgrade(w, r, nil); err == nil {
				defer ws.Close()
				for {
					if _, _, err := ws.ReadMessage(); err != nil {
						return
					}
				}
			}
			return
		}
		seen <- handshake{r.URL.Path, r.URL.Query().Get("port"), r.URL.Query().Get("mode"), r.Header.Get("Authorization")}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			kind, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			// Multiplexed frames are a 4-byte connection id followed by the
			// payload; echo data frames back on the same connection.
			if kind == websocket.BinaryMessage && len(data) > 4 {
				if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
					return
				}
			}
		}
	}))
	defer srv.Close()

	mac := connect(t, srv.URL)
	tun, err := mac.StartPortForward(context.Background(), 0, 22, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()
	h := <-seen
	if h != (handshake{"/port-forward", "22", "multiplexed", "Bearer control"}) {
		t.Errorf("handshake = %+v", h)
	}
	if !strings.HasPrefix(tun.Addr(), "127.0.0.1:") {
		t.Errorf("listening on %s, want 127.0.0.1", tun.Addr())
	}
	conn, err := net.Dial("tcp", tun.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ping" {
		t.Errorf("echo = %q", buf)
	}
}

func TestStartPortForwardRejectsBadRemotePort(t *testing.T) {
	srv, _, _ := guest(t)
	if _, err := connect(t, srv.URL).StartPortForward(context.Background(), 0, 0, nil); err == nil {
		t.Error("remote port 0 must fail")
	}
}

func TestCreateClientNeedsToken(t *testing.T) {
	srv, _, _ := guest(t)
	svc := limrun.NewMacosInstanceService()
	if _, err := svc.CreateClient(limrun.MacosInstance{Status: limrun.MacosInstanceStatus{APIURL: srv.URL}}); err == nil {
		t.Error("an instance without a token must fail")
	}
	mac, err := svc.CreateClientWithCredentials(srv.URL, "control")
	if err != nil || mac == nil {
		t.Fatalf("CreateClientWithCredentials = %v, %v", mac, err)
	}
	_ = mac.Close()
}

func TestRecordingsCapturesAndTrustedCAs(t *testing.T) {
	srv, requests, signals := guest(t)
	ctx := context.Background()
	mac := connect(t, srv.URL)

	quality, ttl := 7, int64(600)
	id, err := mac.StartRecording(ctx, &macos.RecordingOptions{Quality: &quality, Persist: &macos.PersistOption{TTLSeconds: &ttl}})
	if err != nil || id != "rec-1" {
		t.Fatalf("StartRecording = %q, %v", id, err)
	}
	local := t.TempDir() + "/out.mp4"
	downloadURL, err := mac.StopRecording(ctx, "", &macos.SaveRecordingTo{LocalPath: local})
	if err != nil || downloadURL != srv.URL+"/files?name=recording_rec-1.mp4" {
		t.Fatalf("StopRecording = %q, %v", downloadURL, err)
	}
	if _, err := mac.StopRecording(ctx, "rec-1", &macos.SaveRecordingTo{PresignedURL: "https://bucket.example/put"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mac.StopRecording(ctx, "Not Valid", nil); err == nil {
		t.Error("an invalid recording ID was sent")
	}
	launched, err := mac.LaunchApp(ctx, "Example", &macos.LaunchAppOptions{
		Mode: macos.RelaunchIfRunning, Arguments: []string{"-v"}, Env: map[string]string{"A": "b"},
	})
	if err != nil || *launched != (macos.LaunchAppResult{BundleID: "com.example.app", PID: 42, Launched: true}) {
		t.Fatalf("LaunchApp = %+v, %v", launched, err)
	}
	if err := mac.TerminateApp(ctx, "com.example.app"); err != nil {
		t.Fatal(err)
	}
	logs, err := mac.AppLogTail(ctx, "com.apple.TextEdit", 50)
	if err != nil || logs != "one\ntwo" {
		t.Fatalf("AppLogTail = %q, %v", logs, err)
	}
	for _, call := range []func() error{
		func() error { return mac.StartAppLogCapture(ctx, "com.apple.TextEdit", &macos.PersistOption{}) },
		func() error { return mac.StopAppLogCapture(ctx) },
		func() error { return mac.StartEventCapture(ctx, nil) },
		func() error { return mac.StopEventCapture(ctx) },
		func() error { hour := int64(3600); return mac.StartPerformanceCapture(ctx, &hour) },
		func() error { return mac.StopPerformanceCapture(ctx) },
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	trusted, err := mac.AddCACertificate(ctx, "-----BEGIN CERTIFICATE-----")
	if err != nil || trusted.SHA256 != "abc" {
		t.Fatalf("AddCACertificate = %+v, %v", trusted, err)
	}

	var http []string
	for _, r := range requests() {
		http = append(http, r.Method+" "+r.Path)
	}
	if want := []string{"GET /files?name=recording_rec-1.mp4", "POST /trusted-cas"}; !reflect.DeepEqual(http, want) {
		t.Errorf("HTTP requests = %v, want %v", http, want)
	}
	want := []string{
		`{"persist":true,"quality":7,"ttlSeconds":600,"type":"startVideoRecordingSession"}`,
		`{"type":"stopVideoRecordingSession"}`,
		`{"recordingId":"rec-1","type":"stopVideoRecordingSession","upload":{"presignedUrl":"https://bucket.example/put"}}`,
		`{"args":["-v"],"bundleId":"Example","env":{"A":"b"},"mode":"RelaunchIfRunning","type":"launchApp"}`,
		`{"bundleId":"com.example.app","type":"terminateApp"}`,
		`{"bundleId":"com.apple.TextEdit","lines":50,"type":"appLogTail"}`,
		`{"bundleId":"com.apple.TextEdit","persist":true,"type":"startAppLogCapture"}`,
		`{"type":"stopAppLogCapture"}`,
		`{"type":"startEventCapture"}`,
		`{"type":"stopEventCapture"}`,
		`{"ttlSeconds":3600,"type":"startPerformanceCapture"}`,
		`{"type":"stopPerformanceCapture"}`,
	}
	if got := signals(); !reflect.DeepEqual(got, want) {
		t.Errorf("signals =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestListSessionArtifactsByKind(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	client := limrun.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("key"))
	ctx := context.Background()
	for _, list := range []func(context.Context, string, ...option.RequestOption) ([]session.Artifact, error){
		client.MacosInstances.ListRecordings, client.MacosInstances.ListAppLogs,
		client.MacosInstances.ListEvents, client.MacosInstances.ListPerformanceCaptures,
	} {
		if _, err := list(ctx, "macos_eu_01h"); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"/v1/macos_instances/macos_eu_01h/session_artifacts?kind=recording",
		"/v1/macos_instances/macos_eu_01h/session_artifacts?kind=appLog",
		"/v1/macos_instances/macos_eu_01h/session_artifacts?kind=eventLog",
		"/v1/macos_instances/macos_eu_01h/session_artifacts?kind=performance",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v", paths)
	}
}
