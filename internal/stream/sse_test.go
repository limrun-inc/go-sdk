package stream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSSELineEndingsAndFields(t *testing.T) {
	for _, delimiter := range []string{"\n", "\r\n", "\r"} {
		raw := strings.Join([]string{"\ufeff: heartbeat", "retry: 7", "id: 12", "event: stdout", "data: first", "data: second", "", "id: bad\x00id", "data: next", "", "data: incomplete"}, delimiter)
		var got []Event
		var retry time.Duration
		err := Read(strings.NewReader(raw), func(e Event) error { got = append(got, e); return nil }, func() {}, func(d time.Duration) { retry = d })
		want := []Event{{ID: "12", Type: "stdout", Data: "first\nsecond"}, {ID: "12", Type: "message", Data: "next"}}
		if !errors.Is(err, io.EOF) || !reflect.DeepEqual(got, want) || retry != 7*time.Millisecond {
			t.Fatalf("delimiter=%q events=%+v retry=%s err=%v", delimiter, got, retry, err)
		}
	}
}
func TestSSEResumesAfterEOF(t *testing.T) {
	var requests atomic.Int32
	stop := errors.New("done")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Error("authorization missing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			_, _ = io.WriteString(w, "id: 41\ndata: first\n\n")
			return
		}
		if r.Header.Get("Last-Event-ID") != "41" {
			t.Errorf("last event=%q", r.Header.Get("Last-Event-ID"))
		}
		_, _ = io.WriteString(w, "id: 42\ndata: second\n\n")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	count := 0
	err := Run(ctx, Options{URL: server.URL, Token: "test", Retry: time.Millisecond, OnEvent: func(e Event) error {
		count++
		if e.ID == "42" {
			return stop
		}
		return nil
	}})
	if !errors.Is(err, stop) || count != 2 {
		t.Fatalf("events=%d error=%v", count, err)
	}
}
