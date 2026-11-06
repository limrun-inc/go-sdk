package syncfolder

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type PreparedAppBundle struct {
	AppPath, CacheIdentityPath, ArchivePath string
	IsArchive                               bool
}

func PrepareAppBundlePath(input string) (PreparedAppBundle, error) {
	absolute, err := filepath.Abs(input)
	if err != nil {
		return PreparedAppBundle{}, err
	}
	stat, err := os.Stat(absolute)
	if err != nil {
		return PreparedAppBundle{}, err
	}
	if stat.IsDir() {
		return PreparedAppBundle{AppPath: absolute, CacheIdentityPath: absolute}, nil
	}
	if !stat.Mode().IsRegular() {
		return PreparedAppBundle{}, fmt.Errorf("the path is neither an app bundle directory nor a ZIP/IPA archive: %s", input)
	}
	return ExtractAppArchiveToStablePath(absolute)
}

func unsafeArchivePath(name string) bool {
	if strings.ContainsRune(name, 0) || strings.HasPrefix(name, "/") || filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return true
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func ExtractAppArchiveToStablePath(input string) (PreparedAppBundle, error) {
	absolute, err := filepath.Abs(input)
	if err != nil {
		return PreparedAppBundle{}, err
	}
	reader, err := zip.OpenReader(absolute)
	if err != nil {
		return PreparedAppBundle{}, fmt.Errorf("the path is not a valid app bundle directory or ZIP/IPA archive: %s: %w", input, err)
	}
	defer reader.Close()
	apps := map[string]bool{}
	for _, entry := range reader.File {
		name := strings.ReplaceAll(entry.Name, "\\", "/")
		if unsafeArchivePath(name) {
			return PreparedAppBundle{}, fmt.Errorf("ZIP entry has an unsafe path: %s", entry.Name)
		}
		if strings.HasSuffix(name, "/") || !strings.HasPrefix(name, "Payload/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(name, "Payload/"), "/")
		if len(parts) < 2 || !strings.HasSuffix(parts[0], ".app") {
			continue
		}
		apps[parts[0]] = apps[parts[0]] || strings.Join(parts[1:], "/") == "Info.plist"
	}
	if len(apps) != 1 {
		return PreparedAppBundle{}, fmt.Errorf("ZIP/IPA archive must contain exactly one Payload/*.app bundle; found %d", len(apps))
	}
	var appName string
	for name, hasPlist := range apps {
		if !hasPlist {
			return PreparedAppBundle{}, fmt.Errorf("ZIP/IPA archive app bundle is missing Info.plist: Payload/%s/Info.plist", name)
		}
		appName = name
	}
	hash := sha1.Sum([]byte(absolute))
	base := strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))
	base = strings.Trim(regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(base, "-"), "-")
	if base == "" {
		base = "app"
	}
	root := filepath.Join(os.TempDir(), "limrun-sync-app-"+base+"-"+hex.EncodeToString(hash[:6]))
	if stat, e := os.Lstat(root); e == nil && (!stat.IsDir() || stat.Mode()&os.ModeSymlink != 0) {
		if err = os.RemoveAll(root); err != nil {
			return PreparedAppBundle{}, err
		}
	}
	appPath := filepath.Join(root, "Extracted.app")
	if err = os.RemoveAll(appPath); err != nil {
		return PreparedAppBundle{}, err
	}
	if err = os.MkdirAll(appPath, 0755); err != nil {
		return PreparedAppBundle{}, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(appPath)
	if err != nil {
		return PreparedAppBundle{}, err
	}
	prefix := "Payload/" + appName + "/"
	for _, entry := range reader.File {
		name := strings.ReplaceAll(entry.Name, "\\", "/")
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(name, prefix)
		if rel == "" {
			continue
		}
		target := filepath.Join(appPath, filepath.FromSlash(rel))
		// Resolve existing symlink parents before writing through them.
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return PreparedAppBundle{}, err
		}
		parent, e := filepath.EvalSymlinks(filepath.Dir(target))
		if e != nil {
			return PreparedAppBundle{}, e
		}
		if parent != resolvedRoot && !strings.HasPrefix(parent, resolvedRoot+string(filepath.Separator)) {
			return PreparedAppBundle{}, fmt.Errorf("ZIP entry escapes app bundle: %s", name)
		}
		mode := entry.Mode()
		perm := mode.Perm()
		if strings.HasSuffix(name, "/") {
			if perm == 0 {
				perm = 0755
			}
			err = os.MkdirAll(target, perm)
		} else {
			var in io.ReadCloser
			in, err = entry.Open()
			if err != nil {
				return PreparedAppBundle{}, err
			}
			if mode&os.ModeSymlink != 0 {
				var data []byte
				data, err = io.ReadAll(in)
				if err == nil {
					link := string(data)
					resolved := filepath.Clean(filepath.Join(filepath.Dir(target), link))
					if filepath.IsAbs(link) {
						resolved = filepath.Clean(link)
					}
					if resolved != appPath && !strings.HasPrefix(resolved, appPath+string(filepath.Separator)) {
						err = fmt.Errorf("ZIP/IPA symlink escapes app bundle: %s -> %s", name, link)
					} else {
						if e = os.Remove(target); e != nil && !os.IsNotExist(e) {
							err = e
						} else {
							err = os.Symlink(link, target)
						}
					}
				}
			} else {
				if perm == 0 {
					perm = 0644
				}
				if stat, e := os.Lstat(target); e == nil && stat.Mode()&os.ModeSymlink != 0 {
					err = os.Remove(target)
				}
				if err == nil {
					var out *os.File
					out, err = os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
					if err == nil {
						_, err = io.Copy(out, in)
						err = errors.Join(err, out.Close())
						if err == nil {
							err = os.Chmod(target, perm)
						}
					}
				}
			}
			in.Close()
		}
		if err != nil {
			return PreparedAppBundle{}, err
		}
	}
	return PreparedAppBundle{AppPath: appPath, CacheIdentityPath: absolute, ArchivePath: absolute, IsArchive: true}, nil
}

type ArchiveWatchOptions struct {
	Log         func(string, string)
	OnExtracted func() error
}
type ArchiveWatcher struct {
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func (w *ArchiveWatcher) Close() error { w.once.Do(w.cancel); <-w.done; return nil }

func WatchAppArchive(ctx context.Context, archivePath string, opts ArchiveWatchOptions) (*ArchiveWatcher, error) {
	absolute, err := filepath.Abs(archivePath)
	if err != nil {
		return nil, err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err = w.Add(filepath.Dir(absolute)); err != nil {
		w.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	watcher := &ArchiveWatcher{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(watcher.done)
		defer w.Close()
		var timer *time.Timer
		var tick <-chan time.Time
		var active <-chan error
		queued := false
		start := func() {
			if active != nil {
				queued = true
				return
			}
			result := make(chan error, 1)
			active = result
			go func() {
				var previous os.FileInfo
				deadline := time.Now().Add(10 * time.Second)
				for time.Now().Before(deadline) && ctx.Err() == nil {
					stat, e := os.Stat(absolute)
					if e != nil && !os.IsNotExist(e) {
						result <- e
						return
					}
					if e == nil && stat.Mode().IsRegular() {
						if previous != nil && previous.Size() == stat.Size() && previous.ModTime() == stat.ModTime() {
							_, e = ExtractAppArchiveToStablePath(absolute)
							if e == nil && opts.OnExtracted != nil {
								e = opts.OnExtracted()
							}
							result <- e
							return
						}
						previous = stat
					} else {
						previous = nil
					}
					select {
					case <-ctx.Done():
						result <- nil
						return
					case <-time.After(100 * time.Millisecond):
					}
				}
				result <- fmt.Errorf("ZIP/IPA archive did not become readable after change: %s", absolute)
			}()
		}
		for {
			select {
			case <-ctx.Done():
				if timer != nil {
					timer.Stop()
				}
				if active != nil {
					<-active
				}
				return
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if event.Name != absolute {
					continue
				}
				if timer != nil {
					timer.Stop()
				}
				timer = time.NewTimer(500 * time.Millisecond)
				tick = timer.C
			case err := <-w.Errors:
				if err != nil && opts.Log != nil {
					opts.Log("error", err.Error())
				}
			case <-tick:
				tick = nil
				start()
			case err := <-active:
				active = nil
				if err != nil && opts.Log != nil {
					opts.Log("error", err.Error())
				}
				if queued {
					queued = false
					start()
				}
			}
		}
	}()
	return watcher, nil
}
