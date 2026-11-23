package limrun_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
)

func snapshotFrame(w http.ResponseWriter, event, restore, save string) {
	state := map[string]any{"restore": map[string]string{"phase": restore}, "save": map[string]string{"phase": save, "key": "app"}}
	data, _ := json.Marshal(state)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	w.(http.Flusher).Flush()
}

func TestFollowGradleSnapshotReportsEachRestorePhaseOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/gradle_instances/gradle_test/snapshot" || r.Header.Get("Accept") != "text/event-stream" {
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
	result, err := limrun.FollowGradleSnapshot(context.Background(), limrun.GradleSnapshotFollowTarget{BaseURL: server.URL, APIKey: "key", InstanceID: "gradle_test"},
		limrun.GradleSnapshotFollowOptions{Timeout: time.Second, OnUpdate: func(s limrun.GradleInstanceSnapshot) { updates = append(updates, s.Restore.Phase) }})
	if err != nil {
		t.Fatal(err)
	}
	if result.Gone || result.Snapshot.Restore.Phase != "restored" || !reflect.DeepEqual(updates, []string{"pending", "downloading", "extracting", "restored"}) {
		t.Fatalf("result=%+v updates=%v", result, updates)
	}
}

func TestFollowGradleSnapshotWaitsForTheSave(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		snapshotFrame(w, "snapshot", "restored", "requested")
		snapshotFrame(w, "snapshot", "restored", "uploading")
		snapshotFrame(w, "snapshot", "restored", "published")
	}))
	defer server.Close()
	c := limrun.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("key"))
	result, err := c.GradleInstances.FollowSnapshot(context.Background(), "gradle_test", limrun.GradleSnapshotFollowOptions{Side: "save", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Save.Phase != "published" || result.Snapshot.Save.Key != "app" {
		t.Fatalf("result=%+v", result)
	}
}

func TestFollowGradleSnapshotGoneAndTimeoutErrors(t *testing.T) {
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
			_, err := c.GradleInstances.FollowSnapshot(context.Background(), "gradle_test", limrun.GradleSnapshotFollowOptions{Timeout: 50 * time.Millisecond})
			var gone *limrun.GradleSnapshotGoneError
			var timed *limrun.GradleSnapshotTimeoutError
			if status == 404 {
				if !errors.As(err, &gone) {
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
func TestGradleSnapshotRequests(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.RequestURI()+" "+strings.TrimSpace(string(body)))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/gradle_instances":
			_, _ = fmt.Fprint(w, `{"metadata":{"id":"gradle_test","organizationId":"org","createdAt":"2026-11-20T00:00:00Z"},"spec":{"region":"eu","inactivityTimeout":"10m","snapshot":{"key":"app","restoreKeys":["app"]}},"status":{"state":"ready","snapshot":{"restoreKey":"app"}}}`)
		case strings.HasSuffix(r.URL.Path, "/snapshot"):
			_, _ = fmt.Fprint(w, `{"config":{"key":"app"},"restore":{"phase":"restored","matchedKey":"app","source":"tag","bytes":42},"save":{"phase":"idle","key":"app"}}`)
		default:
			_, _ = fmt.Fprint(w, `{"snapshots":[{"key":"app","compatKey":"v1","archiveSizeBytes":42}],"nextOffset":25}`)
		}
	}))
	defer server.Close()
	c := limrun.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("key"))
	ctx := context.Background()

	instance, err := c.GradleInstances.New(ctx, limrun.GradleInstanceNewParams{Spec: limrun.GradleInstanceNewParamsSpec{
		Snapshot: &limrun.GradleSnapshotConfig{Key: limrun.String("app"), RestoreKeys: []string{"app"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if instance.Spec.Snapshot.Key != "app" || instance.Status.Snapshot.RestoreKey != "app" {
		t.Fatalf("instance spec=%+v status=%+v", instance.Spec.Snapshot, instance.Status.Snapshot)
	}
	bound, err := c.GradleInstances.BindSnapshotKey(ctx, "gradle_test", "app")
	if err != nil {
		t.Fatal(err)
	}
	if bound.Restore.Source != "tag" || bound.Restore.Bytes == nil || *bound.Restore.Bytes != 42 || bound.Save.Key != "app" {
		t.Fatalf("bound=%+v", bound)
	}
	list, err := c.GradleSnapshots.List(ctx, "org_test", limrun.SnapshotListParams{Prefix: limrun.String("app")})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Snapshots) != 1 || list.Snapshots[0].CompatKey != "v1" || list.NextOffset == nil || *list.NextOffset != 25 {
		t.Fatalf("list=%+v", list)
	}
	want := []string{
		`POST /v1/gradle_instances {"spec":{"snapshot":{"key":"app","restoreKeys":["app"]}}}`,
		`PUT /v1/gradle_instances/gradle_test/snapshot {"key":"app"}`,
		`GET /v1/organizations/org_test/gradle_snapshots?prefix=app `,
	}
	for i := range want {
		want[i] = strings.TrimSpace(want[i])
		if i >= len(seen) || strings.TrimSpace(seen[i]) != want[i] {
			t.Fatalf("requests:\n%s\nwant:\n%s", strings.Join(seen, "\n"), strings.Join(want, "\n"))
		}
	}
}
