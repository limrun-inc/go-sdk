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
