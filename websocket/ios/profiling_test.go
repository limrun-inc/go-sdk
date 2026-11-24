package ios

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// profilingClient talks to a stand-in instance that records each request body by path.
func profilingClient(t *testing.T, status int, answers map[string]string) (*Client, map[string]map[string]any) {
	t.Helper()
	bodies := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer t" {
			t.Errorf("request = %s with Authorization %q", r.Method, r.Header.Get("Authorization"))
		}
		data, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(data, &body)
		bodies[r.URL.Path] = body
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answers[r.URL.Path])
	}))
	t.Cleanup(srv.Close)
	return &Client{apiURL: srv.URL, token: "t", httpClient: http.DefaultClient}, bodies
}

func TestSampleStacksAndMemoryReport(t *testing.T) {
	c, bodies := profilingClient(t, http.StatusOK, map[string]string{
		"/performance/stacks": `{"bundleId":"com.example.app","pid":7,"durationSeconds":3,"report":"Call graph:\n"}`,
		"/performance/memory": `{"bundleId":"com.example.app","pid":7,"footprintBytes":10,"peakFootprintBytes":12,` +
			`"leakCount":0,"leakedBytes":0,"leaks":"0 leaks","heap":"COUNT","regions":"Physical footprint"}`,
	})

	sample, err := c.SampleStacks(context.Background(), &SampleStacksOptions{BundleID: "com.example.app", DurationSeconds: 3})
	if err != nil {
		t.Fatal(err)
	}
	if *sample != (StackSample{BundleID: "com.example.app", PID: 7, DurationSeconds: 3, Report: "Call graph:\n"}) {
		t.Fatalf("sample = %+v", *sample)
	}
	if body := bodies["/performance/stacks"]; body["bundleId"] != "com.example.app" || body["durationSeconds"] != float64(3) {
		t.Fatalf("stacks request = %v", body)
	}

	// Without options the instance picks the app launched last.
	report, err := c.MemoryReport(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// No leaks is a count of zero, which differs from totals the tool did not print.
	if report.BundleID != "com.example.app" || report.FootprintBytes != 10 || report.PeakFootprintBytes != 12 ||
		report.LeakCount == nil || *report.LeakCount != 0 || report.Heap != "COUNT" || report.Regions != "Physical footprint" {
		t.Fatalf("report = %+v", *report)
	}
	if body := bodies["/performance/memory"]; len(body) != 0 {
		t.Fatalf("memory request = %v, want no fields", body)
	}

	if _, err := c.SampleStacks(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if body := bodies["/performance/stacks"]; body["durationSeconds"] != float64(5) || body["bundleId"] != nil {
		t.Fatalf("default stacks request = %v, want 5 seconds and no bundle", body)
	}
}

func TestSampleStacksRefusesDurationOutOfRange(t *testing.T) {
	c, bodies := profilingClient(t, http.StatusOK, nil)
	for _, seconds := range []int{-1, 31} {
		if _, err := c.SampleStacks(context.Background(), &SampleStacksOptions{DurationSeconds: seconds}); err == nil {
			t.Fatalf("DurationSeconds %d was accepted", seconds)
		}
	}
	if len(bodies) != 0 {
		t.Fatalf("the instance was asked: %v", bodies)
	}
}

// A runtime from before these operations has no such route, which is the one case a caller can
// fix by creating a new instance.
func TestProfilingOnAnOlderRuntime(t *testing.T) {
	c, _ := profilingClient(t, http.StatusNotFound, nil)
	if _, err := c.SampleStacks(context.Background(), nil); !errors.Is(err, ErrAppProfilingUnavailable) {
		t.Fatalf("SampleStacks error = %v, want ErrAppProfilingUnavailable", err)
	}
	if _, err := c.MemoryReport(context.Background(), nil); !errors.Is(err, ErrAppProfilingUnavailable) {
		t.Fatalf("MemoryReport error = %v, want ErrAppProfilingUnavailable", err)
	}
}

func TestProfilingRefusalCarriesTheInstanceMessage(t *testing.T) {
	c, _ := profilingClient(t, http.StatusUnprocessableEntity, map[string]string{
		"/performance/memory": `{"message":"com.example.app is not running."}`,
	})
	_, err := c.MemoryReport(context.Background(), &MemoryReportOptions{BundleID: "com.example.app"})
	if err == nil || !strings.Contains(err.Error(), "com.example.app is not running.") || !strings.Contains(err.Error(), "422") {
		t.Fatalf("error = %v, want the instance message and status", err)
	}
}
