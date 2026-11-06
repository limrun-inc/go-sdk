// Package syncfolder uploads source trees and app bundles using the daemon's delta-sync protocol.
package syncfolder

import (
	"compress/gzip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/klauspost/compress/zstd"
	"github.com/limrun-inc/go-sdk/internal/control"
	"github.com/limrun-inc/go-sdk/internal/direct"
	"github.com/limrun-inc/go-sdk/internal/proxy"
)

type LaunchMode string

const (
	LaunchNone                LaunchMode = "None"
	LaunchForegroundIfRunning LaunchMode = "ForegroundIfRunning"
	LaunchRelaunchIfRunning   LaunchMode = "RelaunchIfRunning"
)

type AdditionalFile struct{ LocalPath, RemotePath string }
type Options struct {
	APIURL, Token, UDID, BasisCacheDir string
	HTTPClient                         *http.Client
	Install                            *bool
	LaunchMode                         LaunchMode
	Watch, SyncSymlinks                bool
	Ignore                             func(string) bool
	AdditionalFiles                    []AdditionalFile
	Compression                        string
	Log                                func(string, string)
	OnSyncComplete                     func(CompleteEvent)
}
type CompleteEvent struct {
	BytesSent                           int64
	DurationMs                          float64
	InstalledAppPath, InstalledBundleID string
}
type Result struct {
	BytesSent                           int64
	InstalledAppPath, InstalledBundleID string
	StopWatching                        func() error
}
type entry struct {
	Path   string  `json:"path"`
	Size   int64   `json:"size"`
	SHA256 string  `json:"sha256"`
	Mode   *uint32 `json:"mode,omitempty"`
	Link   *string `json:"link,omitempty"`
	local  string
}
type payload struct {
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	BasisSHA256  string `json:"basisSha256,omitempty"`
	TargetSHA256 string `json:"targetSha256"`
	Length       int64  `json:"length"`
	local        string
}
type manifest struct {
	ID         string     `json:"id"`
	RootName   string     `json:"rootName"`
	Install    *bool      `json:"install,omitempty"`
	LaunchMode LaunchMode `json:"launchMode,omitempty"`
	Files      []entry    `json:"files"`
	Payloads   []payload  `json:"payloads"`
}
type response struct {
	OK               bool     `json:"ok"`
	NeedFull         []string `json:"needFull"`
	InstalledAppPath string   `json:"installedAppPath"`
	BundleID         string   `json:"bundleId"`
	Error            string   `json:"error"`
}

func BasisCache(local, override string) (key, dir string, err error) {
	absolute, err := filepath.Abs(local)
	if err != nil {
		return "", "", err
	}
	hash := sha1.Sum([]byte(absolute))
	key = "limsync-cache-" + filepath.Base(absolute) + "-" + hex.EncodeToString(hash[:4])
	dir = override
	if dir == "" {
		dir = filepath.Join(os.TempDir(), key)
	}
	return
}
func hashFile(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func walk(root string, opts Options) ([]entry, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	files := []entry{}
	add := func(local, rel string, info fs.FileInfo) error {
		if opts.Ignore != nil && opts.Ignore(rel) {
			return nil
		}
		hash, err := hashFile(local)
		if err != nil {
			return err
		}
		mode := uint32(info.Mode().Perm())
		if info.Mode()&os.ModeSetuid != 0 {
			mode |= 04000
		}
		if info.Mode()&os.ModeSetgid != 0 {
			mode |= 02000
		}
		if info.Mode()&os.ModeSticky != 0 {
			mode |= 01000
		}
		files = append(files, entry{Path: rel, Size: info.Size(), SHA256: hash, Mode: &mode, local: local})
		return nil
	}
	if stat.Mode().IsRegular() {
		if err = add(root, filepath.Base(root), stat); err != nil {
			return nil, err
		}
	} else {
		err = filepath.WalkDir(root, func(local string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if local == root {
				return nil
			}
			rel, _ := filepath.Rel(root, local)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if opts.Ignore != nil && opts.Ignore(rel+"/") {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				if !opts.SyncSymlinks {
					return nil
				}
				if opts.Ignore != nil && (opts.Ignore(rel) || opts.Ignore(rel+"/")) {
					return nil
				}
				target, err := os.Readlink(local)
				if err != nil {
					return err
				}
				target = filepath.ToSlash(target)
				if path.IsAbs(target) || (len(target) > 2 && target[1] == ':') {
					if opts.Log != nil {
						opts.Log("warn", fmt.Sprintf("skipping symlink %s -> %s: absolute targets cannot resolve remotely", rel, target))
					}
					return nil
				}
				resolved := path.Clean(path.Join(path.Dir(rel), target))
				if strings.Contains(target, "\\") || resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") {
					return fmt.Errorf("symlink %s -> %s points outside the sync root or contains a backslash", rel, target)
				}
				hash := sha256.Sum256([]byte(target))
				files = append(files, entry{Path: rel, Size: int64(len(target)), SHA256: hex.EncodeToString(hash[:]), Link: &target, local: local})
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			return add(local, rel, info)
		})
		if err != nil {
			return nil, err
		}
	}
	for _, additional := range opts.AdditionalFiles {
		local, err := filepath.Abs(additional.LocalPath)
		if err != nil {
			return nil, err
		}
		stat, err := os.Stat(local)
		if err != nil {
			return nil, err
		}
		if !stat.Mode().IsRegular() {
			return nil, fmt.Errorf("additional file localPath must be a file: %s", local)
		}
		rel := path.Clean(additional.RemotePath)
		hash, err := hashFile(local)
		if err != nil {
			return nil, err
		}
		mode := uint32(stat.Mode().Perm())
		files = append(files, entry{Path: rel, Size: stat.Size(), SHA256: hash, Mode: &mode, local: local})
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}
func batch(ctx context.Context, opts Options, meta manifest, compression string) (response, error) {
	var result response
	data, err := json.Marshal(meta)
	if err != nil {
		return result, err
	}
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		var err error
		var output io.Writer = writer
		var compressor io.WriteCloser
		switch compression {
		case "gzip":
			compressor, err = gzip.NewWriterLevel(writer, 6)
		case "zstd":
			compressor, err = zstd.NewWriter(writer, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)))
		case "identity":
		default:
			err = fmt.Errorf("unsupported sync compression %q", compression)
		}
		if err == nil && compressor != nil {
			output = compressor
		}
		if err == nil {
			var header [4]byte
			binary.BigEndian.PutUint32(header[:], uint32(len(data)))
			_, err = output.Write(header[:])
			if err == nil {
				_, err = output.Write(data)
			}
		}
		if err == nil {
			for _, p := range meta.Payloads {
				f, e := os.Open(p.local)
				if e != nil {
					err = e
					break
				}
				_, e = io.Copy(output, f)
				f.Close()
				if e != nil {
					err = e
					break
				}
			}
		}
		if compressor != nil {
			if e := compressor.Close(); err == nil {
				err = e
			}
		}
		writer.CloseWithError(err)
		done <- err
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(opts.APIURL, "/")+"/sync", reader)
	if err != nil {
		reader.CloseWithError(err)
		<-done
		return result, err
	}
	req.Header.Set("Authorization", "Bearer "+opts.Token)
	req.Header.Set("Content-Type", "application/octet-stream")
	if compression != "identity" {
		req.Header.Set("Content-Encoding", compression)
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = proxy.Client()
	}
	res, err := hc.Do(req)
	reader.Close()
	streamErr := <-done
	if err != nil {
		if streamErr != nil && !errors.Is(streamErr, io.ErrClosedPipe) {
			return result, streamErr
		}
		return result, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return result, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return result, direct.ResponseError("folder-sync http", res, string(body))
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return result, err
	}
	return result, nil
}
func Sync(ctx context.Context, root string, opts Options) (*Result, error) {
	if opts.BasisCacheDir == "" {
		_, dir, err := BasisCache(root, "")
		if err != nil {
			return nil, err
		}
		opts.BasisCacheDir = dir
	}
	run := func() (*Result, error) {
		start := time.Now()
		result, err := syncOnce(ctx, root, opts)
		if errors.Is(err, os.ErrNotExist) {
			result, err = syncOnce(ctx, root, opts)
		}
		if err == nil && opts.OnSyncComplete != nil {
			opts.OnSyncComplete(CompleteEvent{BytesSent: result.BytesSent, DurationMs: float64(time.Since(start)) / float64(time.Millisecond), InstalledAppPath: result.InstalledAppPath, InstalledBundleID: result.InstalledBundleID})
		}
		return result, err
	}
	result, err := run()
	if err != nil || !opts.Watch {
		return result, err
	}
	stop, err := watch(ctx, root, opts, run)
	if err != nil {
		return nil, err
	}
	result.StopWatching = stop
	return result, nil
}
func syncOnce(ctx context.Context, root string, opts Options) (*Result, error) {
	files, err := walk(root, opts)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(opts.BasisCacheDir, 0755); err != nil {
		return nil, err
	}
	temp, err := os.MkdirTemp("", "limrun-delta-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	payloads := []payload{}
	bytesSent := int64(0)
	hasDelta := false
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if f.Link != nil {
			continue
		}
		basis := cachePath(opts.BasisCacheDir, f.Path)
		basisStat, statErr := os.Lstat(basis)
		basisHash := ""
		if statErr == nil && basisStat.Mode().IsRegular() {
			basisHash, err = hashFile(basis)
			if err != nil {
				return nil, err
			}
			if basisHash == f.SHA256 {
				continue
			}
		}
		p := payload{Kind: "full", Path: f.Path, TargetSHA256: f.SHA256, Length: f.Size, local: f.local}
		if basisHash != "" && f.Size >= 64*1024 {
			patch := filepath.Join(temp, control.ID())
			size, err := EncodePatch(basis, f.local, patch, (f.Size*9+9)/10-1)
			if err != nil {
				return nil, err
			}
			if size >= 0 {
				p = payload{Kind: "delta", Path: f.Path, BasisSHA256: basisHash, TargetSHA256: f.SHA256, Length: size, local: patch}
				hasDelta = true
			}
		}
		payloads = append(payloads, p)
		bytesSent += p.Length
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	meta := manifest{ID: control.ID(), RootName: filepath.Base(absolute), Install: opts.Install, LaunchMode: opts.LaunchMode, Files: files, Payloads: payloads}
	compression := opts.Compression
	if compression == "" {
		compression = "zstd"
		if hasDelta {
			compression = "identity"
		}
	}
	res, err := batch(ctx, opts, meta, compression)
	if err != nil {
		return nil, err
	}
	if !res.OK && len(res.NeedFull) > 0 {
		wanted := make(map[string]bool)
		for _, name := range res.NeedFull {
			wanted[name] = true
		}
		retry := []payload{}
		for _, f := range files {
			if !wanted[f.Path] {
				continue
			}
			if f.Link != nil {
				return nil, fmt.Errorf("this instance's build daemon does not support symlinks yet: %s", f.Path)
			}
			retry = append(retry, payload{Kind: "full", Path: f.Path, TargetSHA256: f.SHA256, Length: f.Size, local: f.local})
			bytesSent += f.Size
		}
		if len(retry) > 0 {
			meta.ID = control.ID()
			meta.Payloads = retry
			compression = opts.Compression
			if compression == "" {
				compression = "zstd"
			}
			res, err = batch(ctx, opts, meta, compression)
			if err != nil {
				return nil, err
			}
		}
	}
	if !res.OK {
		if res.Error == "" {
			res.Error = "sync failed"
		}
		return nil, errors.New(res.Error)
	}
	for _, f := range files {
		if err = cachePut(opts.BasisCacheDir, f); err != nil {
			return nil, err
		}
	}
	return &Result{BytesSent: bytesSent, InstalledAppPath: res.InstalledAppPath, InstalledBundleID: res.BundleID}, nil
}
func cachePath(root, rel string) string {
	if path.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
		hash := sha256.Sum256([]byte(rel))
		return filepath.Join(root, ".external", hex.EncodeToString(hash[:]))
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}
func cachePut(root string, f entry) error {
	dst := cachePath(root, f.Path)
	relative, err := filepath.Rel(root, filepath.Dir(dst))
	if err != nil {
		return err
	}
	ancestor := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "." {
			continue
		}
		ancestor = filepath.Join(ancestor, part)
		if info, e := os.Lstat(ancestor); e == nil && !info.IsDir() {
			if e = os.RemoveAll(ancestor); e != nil {
				return e
			}
		}
		if e := os.MkdirAll(ancestor, 0755); e != nil {
			return e
		}
	}
	if info, e := os.Lstat(dst); e == nil && (info.Mode()&os.ModeSymlink != 0 || info.IsDir() || f.Link != nil) {
		if err = os.RemoveAll(dst); err != nil {
			return err
		}
	}
	if f.Link != nil {
		return os.Symlink(*f.Link, dst)
	}
	source, err := os.Open(f.local)
	if err != nil {
		return err
	}
	defer source.Close()
	output, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, source)
	closeErr := output.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func watch(ctx context.Context, root string, opts Options, run func() (*Result, error)) (func() error, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		w.Close()
		return nil, err
	}
	stat, err := os.Stat(absolute)
	if err != nil {
		w.Close()
		return nil, err
	}
	single := stat.Mode().IsRegular()
	addTree := func() error {
		if single {
			return w.Add(filepath.Dir(absolute))
		}
		return filepath.WalkDir(absolute, func(name string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(absolute, name)
			if rel != "." && opts.Ignore != nil && opts.Ignore(filepath.ToSlash(rel)+"/") {
				return filepath.SkipDir
			}
			return w.Add(name)
		})
	}
	if err = addTree(); err != nil {
		w.Close()
		return nil, err
	}
	done := make(chan struct{})
	stop := make(chan struct{})
	var once sync.Once
	var closeErr error
	go func() {
		defer close(done)
		defer w.Close()
		var timer *time.Timer
		var timerCh <-chan time.Time
		running := false
		queued := false
		completed := make(chan struct{}, 1)
		startRun := func() {
			if running {
				queued = true
				return
			}
			running = true
			go func() {
				if _, err := run(); err != nil && opts.Log != nil {
					opts.Log("error", "syncFolder: watch sync failed: "+err.Error())
				}
				completed <- struct{}{}
			}()
		}
		defer func() {
			if timer != nil {
				timer.Stop()
			}
			if running {
				<-completed
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			case e, ok := <-w.Events:
				if !ok {
					return
				}
				if single && filepath.Clean(e.Name) != absolute {
					continue
				}
				rel, _ := filepath.Rel(absolute, e.Name)
				if !single && opts.Ignore != nil && opts.Ignore(filepath.ToSlash(rel)) {
					continue
				}
				if e.Op&fsnotify.Create != 0 && !single {
					if err := addTree(); err != nil && opts.Log != nil {
						opts.Log("warn", err.Error())
					}
				}
				if timer == nil {
					timer = time.NewTimer(500 * time.Millisecond)
				} else {
					timer.Stop()
					timer.Reset(500 * time.Millisecond)
				}
				timerCh = timer.C
			case <-timerCh:
				timerCh = nil
				startRun()
			case <-completed:
				running = false
				if queued {
					queued = false
					startRun()
				}
			}
		}
	}()
	return func() error {
		once.Do(func() { close(stop); <-done; closeErr = w.Close() })
		if errors.Is(closeErr, fsnotify.ErrClosed) {
			return nil
		}
		return closeErr
	}, nil
}
