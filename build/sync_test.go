package build

import (
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// The instance keeps build state under the folder a project syncs into, so the folder name decides
// whether two checkouts of one project can share that state.
func TestSyncFolderName(t *testing.T) {
	write := func(t *testing.T, name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeProject := func(t *testing.T, dir string) string {
		write(t, filepath.Join(dir, "App.swift"), "import SwiftUI\n")
		return dir
	}
	newCheckout := func(t *testing.T) string {
		checkout := filepath.Join(t.TempDir(), "garden-planner-main")
		write(t, filepath.Join(checkout, ".git", "HEAD"), "ref: refs/heads/main\n")
		write(t, filepath.Join(checkout, ".git", "config"), "[remote \"origin\"]\n\turl = git@github.com:acme/garden-planner.git\n")
		return checkout
	}
	// Lays out what `git worktree add` leaves behind: a .git file pointing into the main checkout.
	addWorktree := func(t *testing.T, checkout, name string) string {
		worktree := filepath.Join(filepath.Dir(checkout), name)
		gitDir := filepath.Join(checkout, ".git", "worktrees", name)
		write(t, filepath.Join(gitDir, "commondir"), "../..\n")
		write(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/"+name+"\n")
		write(t, filepath.Join(worktree, ".git"), "gitdir: "+gitDir+"\n")
		return worktree
	}
	syncedFolderNames := func(t *testing.T, platform string, rootName string, projects ...string) []string {
		t.Helper()
		var mu sync.Mutex
		names := []string{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var input io.Reader = r.Body
			switch r.Header.Get("Content-Encoding") {
			case "zstd":
				decoder, err := zstd.NewReader(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				defer decoder.Close()
				input = decoder
			case "gzip":
				decoder, err := gzip.NewReader(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				defer decoder.Close()
				input = decoder
			}
			var prefix [4]byte
			if _, err := io.ReadFull(input, prefix[:]); err != nil {
				t.Error(err)
				return
			}
			var meta struct {
				RootName string `json:"rootName"`
			}
			if err := json.NewDecoder(io.LimitReader(input, int64(binary.BigEndian.Uint32(prefix[:])))).Decode(&meta); err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			names = append(names, meta.RootName)
			mu.Unlock()
			io.Copy(io.Discard, input)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()
		client := newDaemon(srv.URL, "test-token", platform, nil)
		off := false
		for _, project := range projects {
			opts := &SyncOptions{Watch: &off, Install: &off, BasisCacheDir: t.TempDir(), RootName: rootName}
			if _, err := client.Sync(context.Background(), project, opts); err != nil {
				t.Fatal(err)
			}
		}
		mu.Lock()
		defer mu.Unlock()
		return names
	}
	expect := func(t *testing.T, got []string, want ...string) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("synced into %q, want %q", got, want)
		}
	}

	t.Run("a project at the repository root syncs into the same folder from every worktree", func(t *testing.T) {
		checkout := writeProject(t, newCheckout(t))
		first := writeProject(t, addWorktree(t, checkout, "fix-login-7f3a"))
		second := writeProject(t, addWorktree(t, checkout, "new-onboarding"))
		expect(t, syncedFolderNames(t, "xcode", "", checkout, first, second), "garden-planner", "garden-planner", "garden-planner")
	})
	t.Run("a project in a subdirectory keeps its directory name", func(t *testing.T) {
		checkout := newCheckout(t)
		worktree := addWorktree(t, checkout, "fix-login-7f3a")
		main := writeProject(t, filepath.Join(checkout, "apps", "ios"))
		other := writeProject(t, filepath.Join(worktree, "apps", "ios"))
		expect(t, syncedFolderNames(t, "xcode", "", main, other), "ios", "ios")
	})
	t.Run("a project outside Git syncs into a folder named after the local one", func(t *testing.T) {
		project := writeProject(t, filepath.Join(t.TempDir(), "scratch-app"))
		expect(t, syncedFolderNames(t, "xcode", "", project), "scratch-app")
	})
	t.Run("an explicit folder name wins", func(t *testing.T) {
		checkout := writeProject(t, newCheckout(t))
		expect(t, syncedFolderNames(t, "xcode", "pinned", checkout), "pinned")
	})
	t.Run("a Gradle project keeps the local folder name", func(t *testing.T) {
		checkout := writeProject(t, newCheckout(t))
		expect(t, syncedFolderNames(t, "gradle", "", checkout), "garden-planner-main")
	})
}
