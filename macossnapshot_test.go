package limrun_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
)

func TestFollowMacosSnapshotReportsEachRestorePhaseOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/macos_instances/macos_test/snapshot" || r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("request=%s accept=%s", r.URL, r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		snapshotFrame(w, "snapshot", "pending", "idle")
		snapshotFrame(w, "snapshot", "downloading", "idle")
		// A reconnect replays the current state, which is not a transition.
		snapshotFrame(w, "snapshot", "downloading", "idle")
		// Xcode's event is not this stream's, and is ignored.
		snapshotFrame(w, "cache", "failed", "idle")
		snapshotFrame(w, "snapshot", "extracting", "idle")
		snapshotFrame(w, "snapshot", "restored", "idle")
	}))
	defer server.Close()
	var updates []string
	result, err := limrun.FollowMacosSnapshot(context.Background(), limrun.MacosSnapshotFollowTarget{BaseURL: server.URL, APIKey: "key", InstanceID: "macos_test"},
		limrun.MacosSnapshotFollowOptions{Timeout: time.Second, OnUpdate: func(s limrun.MacosInstanceSnapshot) { updates = append(updates, s.Restore.Phase) }})
	if err != nil {
		t.Fatal(err)
	}
	if result.Gone || result.Snapshot.Restore.Phase != "restored" || !reflect.DeepEqual(updates, []string{"pending", "downloading", "extracting", "restored"}) {
		t.Fatalf("result=%+v updates=%v", result, updates)
	}
}

func TestFollowMacosSnapshotWaitsForTheSave(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		snapshotFrame(w, "snapshot", "restored", "requested")
		snapshotFrame(w, "snapshot", "restored", "uploading")
		snapshotFrame(w, "snapshot", "restored", "published")
	}))
	defer server.Close()
	c := limrun.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("key"))
	result, err := c.MacosInstances.FollowSnapshot(context.Background(), "macos_test", limrun.MacosSnapshotFollowOptions{Side: "save", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Save.Phase != "published" || result.Snapshot.Save.Key != "app" {
		t.Fatalf("result=%+v", result)
	}
}

// A gone event is confirmed against the macOS path; a live instance reconnects the stream.
func TestFollowMacosSnapshotConfirmsGoneAndReconnects(t *testing.T) {
	var streams, confirms atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/macos_instances/macos_test/snapshot" {
			t.Errorf("request=%s", r.URL)
		}
		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "retry: 1\n\n")
			if streams.Add(1) == 1 {
				snapshotFrame(w, "gone", "downloading", "idle")
			} else {
				snapshotFrame(w, "snapshot", "restored", "idle")
			}
			return
		}
		confirms.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"restore":{"phase":"downloading"},"save":{"phase":"idle"}}`)
	}))
	defer server.Close()
	c := limrun.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("key"))
	result, err := c.MacosInstances.FollowSnapshot(context.Background(), "macos_test", limrun.MacosSnapshotFollowOptions{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Gone || result.Snapshot.Restore.Phase != "restored" || confirms.Load() != 1 || streams.Load() != 2 {
		t.Fatalf("result=%+v streams=%d confirms=%d", result, streams.Load(), confirms.Load())
	}
}

func TestFollowMacosSnapshotGoneAndTimeoutErrors(t *testing.T) {
	for _, status := range []int{404, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if status == 404 {
					w.WriteHeader(404)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				snapshotFrame(w, "snapshot", "downloading", "idle")
				<-r.Context().Done()
			}))
			defer server.Close()
			c := limrun.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("key"))
			_, err := c.MacosInstances.FollowSnapshot(context.Background(), "macos_test", limrun.MacosSnapshotFollowOptions{Timeout: 50 * time.Millisecond})
			var gone *limrun.GradleSnapshotGoneError
			var timed *limrun.GradleSnapshotTimeoutError
			if status == 404 {
				if !errors.As(err, &gone) || gone.InstanceID != "macos_test" {
					t.Fatalf("err=%v", err)
				}
			} else if !errors.As(err, &timed) || timed.Snapshot == nil || timed.Snapshot.Restore.Phase != "downloading" {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// The create field, the key binding, the snapshots resource and the instance record all use the
// paths and fields the server serves.
func TestMacosSnapshotRequests(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.RequestURI()+" "+strings.TrimSpace(string(body)))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/macos_instances":
			_, _ = fmt.Fprint(w, `{"metadata":{"id":"macos_test","organizationId":"org","createdAt":"2026-11-20T00:00:00Z"},"spec":{"region":"eu","inactivityTimeout":"10m","snapshot":{"key":"app","restoreKeys":["app"]}},"status":{"state":"terminated","snapshot":{"restoreKey":"app","save":{"phase":"published"}}}}`)
		case strings.HasSuffix(r.URL.Path, "/snapshot"):
			_, _ = fmt.Fprint(w, `{"config":{"key":"app"},"restore":{"phase":"restored","matchedKey":"app","source":"tag","bytes":42},"save":{"phase":"idle","key":"app"}}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/macos_snapshots"):
			_, _ = fmt.Fprint(w, `{"snapshots":[{"key":"app","compatKey":"macos-27.2.1","archiveSizeBytes":42,"uncompressedSizeBytes":84}],"nextOffset":25}`)
		default:
			_, _ = fmt.Fprint(w, `{"key":"github.com/acme/app","compatKey":"macos-27.2.1","sourceInstanceId":"macos_test"}`)
		}
	}))
	defer server.Close()
	c := limrun.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("key"))
	ctx := context.Background()

	instance, err := c.MacosInstances.New(ctx, limrun.MacosInstanceNewParams{Spec: limrun.MacosInstanceNewParamsSpec{
		Snapshot: &limrun.MacosSnapshotConfig{Key: limrun.String("app"), RestoreKeys: []string{"app"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if instance.Spec.Snapshot.Key != "app" || instance.Status.Snapshot.RestoreKey != "app" || instance.Status.Snapshot.Save == nil || instance.Status.Snapshot.Save.Phase != "published" {
		t.Fatalf("instance spec=%+v status=%+v", instance.Spec.Snapshot, instance.Status.Snapshot)
	}
	bound, err := c.MacosInstances.BindSnapshotKey(ctx, "macos_test", "app")
	if err != nil {
		t.Fatal(err)
	}
	if bound.Restore.Source != "tag" || bound.Restore.Bytes == nil || *bound.Restore.Bytes != 42 || bound.Save.Key != "app" {
		t.Fatalf("bound=%+v", bound)
	}
	current, err := c.MacosInstances.GetSnapshot(ctx, "macos_test")
	if err != nil {
		t.Fatal(err)
	}
	if current.Config == nil || current.Config.Key != "app" || current.Restore.MatchedKey != "app" {
		t.Fatalf("current=%+v", current)
	}
	list, err := c.MacosSnapshots.List(ctx, "org_test", limrun.SnapshotListParams{Prefix: limrun.String("app"), Limit: limrun.Int(10), Offset: limrun.Int(5)})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Snapshots) != 1 || list.Snapshots[0].CompatKey != "macos-27.2.1" || list.Snapshots[0].UncompressedSizeBytes != 84 || list.NextOffset == nil || *list.NextOffset != 25 {
		t.Fatalf("list=%+v", list)
	}
	snapshot, err := c.MacosSnapshots.Get(ctx, "org_test", "github.com/acme/app")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Key != "github.com/acme/app" || snapshot.SourceInstanceID != "macos_test" {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if err := c.MacosSnapshots.Delete(ctx, "org_test", "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`POST /v1/macos_instances {"spec":{"snapshot":{"key":"app","restoreKeys":["app"]}}}`,
		`PUT /v1/macos_instances/macos_test/snapshot {"key":"app"}`,
		`GET /v1/macos_instances/macos_test/snapshot`,
		`GET /v1/organizations/org_test/macos_snapshots?limit=10&offset=5&prefix=app`,
		`GET /v1/organizations/org_test/macos_snapshots/github.com%2Facme%2Fapp`,
		`DELETE /v1/organizations/org_test/macos_snapshots/github.com%2Facme%2Fapp`,
	}
	for i := range want {
		if i >= len(seen) || strings.TrimSpace(seen[i]) != want[i] || len(seen) != len(want) {
			t.Fatalf("requests:\n%s\nwant:\n%s", strings.Join(seen, "\n"), strings.Join(want, "\n"))
		}
	}
}
