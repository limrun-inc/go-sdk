package macos_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
)

type recorded struct {
	Method, Path, Auth string
	Body               any
}

func guest(t *testing.T) (*httptest.Server, func() []recorded) {
	t.Helper()
	var mu sync.Mutex
	var requests []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("%s %s: invalid JSON body %q", r.Method, r.URL.Path, raw)
			}
		}
		mu.Lock()
		requests = append(requests, recorded{r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), body})
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
		case r.URL.Path == "/ssh":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"user":"limrun","port":22}`))
		case r.URL.Path == "/snapshot":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: snapshot\ndata: {\"phase\":\"resolving\"}\n\n" +
				": keepalive\n\n" +
				"event: snapshot\ndata: {\"phase\":\"downloading\",\"totalBytes\":10,\"doneBytes\":4}\n\n" +
				"event: snapshot\ndata: {\"phase\":\"restored\",\"key\":\"my-mac\",\"matchKind\":\"exact_hit\"}\n\n"))
		case r.URL.Path == "/screenshot.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xff, 0xd8, 0xff})
		case r.URL.Path == "/type" && body.(map[string]any)["text"] == "fail":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":true,"reason":"no session"}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []recorded { mu.Lock(); defer mu.Unlock(); return append([]recorded(nil), requests...) }
}

func TestClientSendsEachRequest(t *testing.T) {
	srv, requests := guest(t)
	ctx := context.Background()
	var stdout, command bytes.Buffer
	mac := macos.NewClient(srv.URL+"/", "control", build.WithExecutionOptions(build.ExecOptions{Stdout: &stdout, Command: &command}))

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
	// Empty fields stay out of the body.
	if _, err := mac.RestoreSnapshot(ctx, macos.RestoreSnapshotOptions{}); err != nil {
		t.Fatal(err)
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

	type call struct {
		Method, Path string
		Body         any
	}
	var got []call
	for _, r := range requests() {
		if r.Auth != "Bearer control" {
			t.Errorf("%s %s: Authorization = %q", r.Method, r.Path, r.Auth)
		}
		got = append(got, call{r.Method, r.Path, r.Body})
	}
	obj := func(s string) any {
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	want := []call{
		{"POST", "/exec", obj(`{"command":"run","commandLine":"sw_vers","cwd":"Desktop","env":["A=b"],"timeoutSeconds":30}`)},
		{"GET", "/exec/run-1/events?follow=true", nil},
		{"POST", "/ssh", obj(`{"publicKey":"ssh-ed25519 AAAA test"}`)},
		{"GET", "/screenshot.jpg", nil},
		{"POST", "/snapshot", obj(`{"key":"my-mac","restoreKeys":["my-mac","base-"]}`)},
		{"POST", "/snapshot", obj(`{}`)},
		{"POST", "/tap", obj(`{"x":10,"y":20,"button":"right"}`)},
		{"POST", "/tap", obj(`{"x":7,"y":8}`)},
		{"POST", "/type", obj(`{"text":"hi"}`)},
		{"POST", "/press-key", obj(`{"key":"q","modifiers":["command"]}`)},
		{"POST", "/press-key", obj(`{"key":"enter"}`)},
		{"POST", "/scroll", obj(`{"dx":0,"dy":300,"x":5,"y":6}`)},
		{"POST", "/scroll", obj(`{"dx":-4,"dy":0}`)},
		{"POST", "/drag", obj(`{"fromX":1,"fromY":2,"toX":3,"toY":4,"durationMs":100}`)},
		{"POST", "/drag", obj(`{"fromX":1,"fromY":2,"toX":3,"toY":4}`)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("requests =\n%+v\nwant\n%+v", got, want)
	}
}

func TestRunDefaultsCwdToHome(t *testing.T) {
	srv, requests := guest(t)
	proc, err := macos.NewClient(srv.URL, "control").Run(context.Background(), "true", nil)
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

func TestErrorsCarryTheServerReason(t *testing.T) {
	srv, _ := guest(t)
	err := macos.NewClient(srv.URL, "control").TypeText(context.Background(), "fail")
	var he *limrun.DirectInstanceHTTPError
	if !errors.As(err, &he) || he.Status != http.StatusBadRequest {
		t.Fatalf("err = %v, want a 400 DirectInstanceHTTPError", err)
	}
	if err.Error() != "POST /type failed: 400 no session" {
		t.Errorf("err = %q", err.Error())
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

	mac := macos.NewClient(srv.URL, "control")
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
	if _, err := macos.NewClient("http://127.0.0.1:1", "control").StartPortForward(context.Background(), 0, 0, nil); err == nil {
		t.Error("remote port 0 must fail")
	}
}

func TestCreateClientNeedsToken(t *testing.T) {
	svc := limrun.NewMacosInstanceService()
	if _, err := svc.CreateClient(limrun.MacosInstance{Status: limrun.MacosInstanceStatus{APIURL: "https://mac.example.test"}}); err == nil {
		t.Error("an instance without a token must fail")
	}
	mac, err := svc.CreateClientWithCredentials("https://mac.example.test", "control")
	if err != nil || mac == nil {
		t.Errorf("CreateClientWithCredentials = %v, %v", mac, err)
	}
}
