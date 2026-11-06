package limrun_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
)

func cacheFrame(w http.ResponseWriter, event, restore, save string) {
	state := map[string]any{"restore": map[string]string{"phase": restore}, "save": map[string]string{"phase": save, "cacheKey": "key"}}
	data, _ := json.Marshal(state)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	w.(http.Flusher).Flush()
}
func TestFollowSnapshotDeduplicatesAndMapsCacheKey(t *testing.T) {
	var opens atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/xcode_instances/sandbox_test/cache" || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("request=%s", r.URL)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		cacheFrame(w, "cache", "restored", "idle")
		cacheFrame(w, "cache", "restored", "idle")
		cacheFrame(w, "cache", "restored", "uploading")
		cacheFrame(w, "cache", "restored", "published")
	}))
	defer server.Close()
	var updates []string
	result, err := limrun.FollowXcodeSnapshot(context.Background(), limrun.XcodeSnapshotFollowTarget{BaseURL: server.URL, APIKey: "key", InstanceID: "sandbox_test"}, limrun.XcodeSnapshotFollowOptions{Side: "save", Timeout: time.Second, OnOpen: func() { opens.Add(1) }, OnUpdate: func(s limrun.XcodeInstanceSnapshot) { updates = append(updates, s.Save.Phase) }})
	if err != nil {
		t.Fatal(err)
	}
	if result.Gone || result.Snapshot.Save.SnapshotKey != "key" || opens.Load() != 1 || !reflect.DeepEqual(updates, []string{"idle", "uploading", "published"}) {
		t.Fatalf("result=%+v updates=%v opens=%d", result, updates, opens.Load())
	}
}
func TestFollowCacheConfirmsGoneAndReconnects(t *testing.T) {
	var streams, confirms atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "retry: 1\n\n")
			n := streams.Add(1)
			if n == 1 {
				cacheFrame(w, "gone", "downloading", "idle")
			} else {
				cacheFrame(w, "cache", "restored", "idle")
			}
			return
		}
		confirms.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"restore":{"phase":"downloading"},"save":{"phase":"idle"}}`)
	}))
	defer server.Close()
	c := limrun.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("key"))
	result, err := c.XcodeInstances.FollowCache(context.Background(), "sandbox_test", limrun.XcodeCacheFollowOptions{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Gone || result.Cache.Restore.Phase != "restored" || confirms.Load() != 1 || streams.Load() != 2 {
		t.Fatalf("result=%+v streams=%d confirms=%d", result, streams.Load(), confirms.Load())
	}
}
func TestFollowSnapshotGoneAndTimeoutErrors(t *testing.T) {
	for _, status := range []int{404, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if status == 404 {
					w.WriteHeader(404)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				cacheFrame(w, "cache", "downloading", "idle")
				<-r.Context().Done()
			}))
			defer server.Close()
			c := limrun.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("key"))
			_, err := c.XcodeInstances.FollowSnapshot(context.Background(), "sandbox_test", limrun.XcodeSnapshotFollowOptions{Timeout: 50 * time.Millisecond})
			var gone *limrun.XcodeSnapshotGoneError
			var timed *limrun.XcodeSnapshotTimeoutError
			if status == 404 {
				if !errors.As(err, &gone) {
					t.Fatalf("err=%v", err)
				}
			} else {
				if !errors.As(err, &timed) || timed.Snapshot == nil || timed.Snapshot.Restore.Phase != "downloading" {
					t.Fatalf("err=%v", err)
				}
			}
		})
	}
}
