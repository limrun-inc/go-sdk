package build

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestXcodebuildSendsHotReload(t *testing.T) {
	var sent map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/exec":
			if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
				t.Error(err)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"execId":"build-test"}`)
		case "/exec/build-test/events":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "id: 1\nevent: exitCode\ndata: 0\n\n")
		default:
			t.Errorf("unexpected URL %s", r.URL)
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	scheme, on := "Demo", true
	p, err := NewXcodeClient(srv.URL, "test").Xcodebuild(ctx, &XcodeProjectConfig{Scheme: &scheme},
		&XcodeBuildOptions{XcodeBuildExecRequest: XcodeBuildExecRequest{HotReload: &on}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if string(sent["hotReload"]) != "true" {
		t.Fatalf("hotReload on the wire = %s, want true", sent["hotReload"])
	}
}

func TestXcodebuildRejectsHotReloadOutsideDebugSimulatorBuilds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a rejected build reached the server: %s", r.URL)
	}))
	defer srv.Close()
	release, device, watch, on := "Release", "iphoneos", "watchsimulator", true
	for name, tc := range map[string]struct {
		settings XcodeProjectConfig
		want     string
	}{
		"Release":         {XcodeProjectConfig{Configuration: &release}, "hotReload is only supported for Debug builds"},
		"device sdk":      {XcodeProjectConfig{SDK: &device}, "hotReload requires the iphonesimulator sdk"},
		"other simulator": {XcodeProjectConfig{SDK: &watch}, "hotReload requires the iphonesimulator sdk"},
	} {
		_, err := NewXcodeClient(srv.URL, "test").Xcodebuild(context.Background(), &tc.settings,
			&XcodeBuildOptions{XcodeBuildExecRequest: XcodeBuildExecRequest{HotReload: &on}})
		if err == nil || err.Error() != tc.want {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}
