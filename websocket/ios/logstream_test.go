package ios

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// appLogServer serves /app-logs/stream as an instance does; any other path,
// such as the signaling socket, is refused, which the SSE stream never needs.
func appLogServer(t *testing.T, write func(w http.ResponseWriter)) (*httptest.Server, chan string) {
	t.Helper()
	seen := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Method + " " + r.URL.RequestURI() + " " + r.Header.Get("Authorization")
		if r.URL.Path != "/api/app-logs/stream" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		write(w)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func TestStreamAppLogFollowsServerSentEvents(t *testing.T) {
	srv, seen := appLogServer(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(": keepalive\n\ndata: one\n\ndata: two\ndata: lines\n\n"))
	})
	c := &Client{apiURL: srv.URL + "/api", token: "tok", httpClient: srv.Client()}
	s, err := c.StreamAppLog(context.Background(), "com.example.app")
	if err != nil {
		t.Fatal(err)
	}
	var got [][]string
	for lines := range s.Lines() {
		got = append(got, lines)
	}
	if want := [][]string{{"one"}, {"two\nlines"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
	if err := <-s.Err(); err != nil {
		t.Errorf("an ended stream reported %v", err)
	}
	if req := <-seen; req != "GET /api/app-logs/stream?bundleId=com.example.app Bearer tok" {
		t.Errorf("request = %q", req)
	}
}

func TestStreamAppLogRefusedIsAnError(t *testing.T) {
	srv, _ := appLogServer(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusForbidden) })
	c := &Client{apiURL: srv.URL, token: "tok", httpClient: srv.Client()}
	if _, err := c.StreamAppLog(context.Background(), "com.example.app"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want the 404", err)
	}
}

func TestStreamAppLogStopEndsWithoutError(t *testing.T) {
	srv, _ := appLogServer(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: one\n\n"))
		w.(http.Flusher).Flush()
		time.Sleep(5 * time.Second)
	})
	c := &Client{apiURL: srv.URL + "/api", token: "tok", httpClient: srv.Client()}
	s, err := c.StreamAppLog(context.Background(), "com.example.app")
	if err != nil {
		t.Fatal(err)
	}
	if lines := <-s.Lines(); !reflect.DeepEqual(lines, []string{"one"}) {
		t.Errorf("lines = %q", lines)
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	for range s.Lines() {
	}
	if err := <-s.Err(); err != nil {
		t.Errorf("Stop reported %v", err)
	}
}
