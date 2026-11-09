package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecStreamsAndStripsClientMetadata(t *testing.T) {
	var streams atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Error("missing auth")
		}
		switch r.URL.Path {
		case "/exec":
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if _, ok := body["additionalMetadata"]; ok {
				t.Error("client metadata sent to daemon")
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"execId":"build-test"}`)
		case "/exec/build-test/events":
			streams.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "id: 1\nevent: stdout\ndata: building\n\nid: 2\nevent: testflight\ndata: invalid-json\n\nid: 3\nevent: xctest\ndata: {\"type\":\"case\",\"testClass\":\"Tests\",\"method\":\"testA\",\"passed\":true,\"durationMs\":1}\n\nid: 4\nevent: exitCode\ndata: 0\n\n")
		default:
			t.Errorf("unexpected URL %s", r.URL)
		}
	}))
	defer srv.Close()
	var stdout bytes.Buffer
	download := "https://download.example/app"
	p := Exec(context.Background(), XcodeBuildExecRequest{Command: "xcodebuild", AdditionalMetadata: &XcodeBuildExecRequestAdditionalMetadata{SignedDownloadURL: &download}}, ExecOptions{APIURL: srv.URL, Token: "test", Stdout: &stdout})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := p.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "SUCCEEDED" || result.ExitCode != 0 || stdout.String() != "building" || result.SignedDownloadURL == nil {
		t.Fatalf("result=%+v output=%s", result, stdout.String())
	}
	if result.Appstore == nil || result.Appstore.State != "unknown" || result.Xctest == nil || len(result.Xctest.Cases) != 1 {
		t.Fatalf("events=%+v", result)
	}
	if streams.Load() != 1 {
		t.Errorf("stream reopened after exit: %d", streams.Load())
	}
}
func TestObserveResolvesAliasBeforeFollow(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(r.URL.Path, "latest") {
			fmt.Fprint(w, "event: meta\ndata: {\"id\":\"build-real\"}\n\n")
		} else {
			fmt.Fprint(w, "event: exitCode\ndata: 2\n\n")
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := ObserveExecLogs(ctx, "latest", ExecOptions{APIURL: srv.URL, Token: "test"}, ExecLogOptions{Follow: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExecID != "build-real" || r.Status != "FAILED" || len(paths) != 2 || paths[1] != "/exec/build-real/events?follow=true" {
		t.Fatalf("result=%+v paths=%v", r, paths)
	}
}

// detachDaemon is a fake build daemon whose event stream blocks after its first
// event until release is closed, so a test can time Detach against following.
func detachDaemon(t *testing.T, streamOpened chan<- struct{}, release <-chan struct{}, execStarted <-chan struct{}) *httptest.Server {
	t.Helper()
	var opened atomic.Bool
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/exec":
			if execStarted != nil {
				<-execStarted
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"execId":"build-detach"}`)
		case "/exec/build-detach/events":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "id: 1\nevent: stdout\ndata: building\n\n")
			w.(http.Flusher).Flush()
			if opened.CompareAndSwap(false, true) {
				close(streamOpened)
			}
			<-release
			fmt.Fprint(w, "id: 2\nevent: exitCode\ndata: 0\n\n")
		default:
			t.Errorf("unexpected URL %s", r.URL)
		}
	}))
}

func TestDetachAfterFollowingStartedStillSettles(t *testing.T) {
	streamOpened := make(chan struct{})
	release := make(chan struct{})
	srv := detachDaemon(t, streamOpened, release, nil)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p := Exec(ctx, XcodeBuildExecRequest{Command: "xcodebuild"}, ExecOptions{APIURL: srv.URL, Token: "test"})
	select {
	case <-streamOpened:
	case <-ctx.Done():
		t.Fatal("event stream never opened")
	}
	id, err := p.Detach(ctx)
	if err != nil || id != "build-detach" {
		t.Fatalf("detach: id=%q err=%v", id, err)
	}
	close(release)
	result, err := p.Wait(ctx)
	if err != nil || result.Status != "SUCCEEDED" {
		t.Fatalf("late detach stopped the follower: result=%+v err=%v", result, err)
	}
}

func TestDetachBeforeFollowingLeavesWaitPending(t *testing.T) {
	streamOpened := make(chan struct{})
	release := make(chan struct{})
	execStarted := make(chan struct{})
	srv := detachDaemon(t, streamOpened, release, execStarted)
	defer srv.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p := Exec(ctx, XcodeBuildExecRequest{Command: "xcodebuild"}, ExecOptions{APIURL: srv.URL, Token: "test"})
	detached := make(chan error, 1)
	go func() {
		_, err := p.Detach(ctx)
		detached <- err
	}()
	// Detach flags the process before POST /exec answers, so no stream opens.
	time.Sleep(50 * time.Millisecond)
	close(execStarted)
	if err := <-detached; err != nil {
		t.Fatal(err)
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer waitCancel()
	if _, err := p.Wait(waitCtx); err == nil {
		t.Fatal("Wait settled although the process was detached before following")
	}
	select {
	case <-streamOpened:
		t.Fatal("event stream opened after an early detach")
	default:
	}
}
