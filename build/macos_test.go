package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMacOSAttachRoundTrip(t *testing.T) {
	var sent map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/macos" || r.Header.Get("Authorization") != "Bearer xcode-token" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
				t.Error(err)
			}
			fmt.Fprint(w, `{"attached":true,"macos":{"apiUrl":"https://mac.example/v1/macos_abc/api","macosInstanceId":"macos_abc"}}`)
		case http.MethodGet:
			fmt.Fprint(w, `{"attached":true,"macos":{"apiUrl":"https://mac.example/v1/macos_abc/api","macosInstanceId":"macos_abc"}}`)
		case http.MethodDelete:
			fmt.Fprint(w, `{"attached":false}`)
		}
	}))
	defer srv.Close()
	c := NewXcodeClient(srv.URL, "xcode-token")
	ctx := context.Background()

	status, err := c.AttachMacOS(ctx, "https://mac.example/v1/macos_abc/api", "mac-token")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Attached || status.Macos == nil || status.Macos.MacosInstanceID == nil || *status.Macos.MacosInstanceID != "macos_abc" {
		t.Fatalf("attach status = %+v", status)
	}
	if sent["apiUrl"] != "https://mac.example/v1/macos_abc/api" || sent["token"] != "mac-token" {
		t.Errorf("attach body = %v", sent)
	}
	if status, err := c.GetMacOS(ctx); err != nil || !status.Attached {
		t.Fatalf("get = %+v, %v", status, err)
	}
	if status, err := c.DetachMacOS(ctx); err != nil || status.Attached {
		t.Fatalf("detach = %+v, %v", status, err)
	}
}

func TestMacOSAttachOnAnOldInstance(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, err := NewXcodeClient(srv.URL, "t").AttachMacOS(context.Background(), "https://mac.example", "t")
	var unsupported *MacOSAttachUnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("err = %v, want MacOSAttachUnsupportedError", err)
	}
}
