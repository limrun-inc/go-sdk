package android

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/limrun-inc/go-sdk/syncfolder"
)

type SyncAppOptions struct {
	Install, Watch          *bool
	BasisCacheDir           string
	LaunchMode              syncfolder.LaunchMode
	OnSyncComplete          func(syncfolder.CompleteEvent)
	OnBasisDownloadProgress func(downloadedBytes, totalBytes int64)
}

func (c *Client) SyncApp(ctx context.Context, apkPath string, options ...SyncAppOptions) (*syncfolder.Result, error) {
	var opts SyncAppOptions
	if len(options) > 0 {
		opts = options[0]
	}
	absolute, err := filepath.Abs(apkPath)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("APK path must be a regular file: %s", absolute)
	}
	f, err := os.Open(absolute)
	if err != nil {
		return nil, err
	}
	var magic [4]byte
	_, err = io.ReadFull(f, magic[:])
	f.Close()
	if err != nil || magic[0] != 'P' || magic[1] != 'K' {
		return nil, fmt.Errorf("file is not a valid APK (ZIP) archive: %s", absolute)
	}
	hash := sha1.Sum([]byte(absolute))
	key := "limsync-cache-android-" + filepath.Base(absolute) + "-" + hex.EncodeToString(hash[:4])
	basis := opts.BasisCacheDir
	if basis == "" {
		basis = filepath.Join(os.TempDir(), key)
	}
	install := true
	if opts.Install != nil {
		install = *opts.Install
	}
	watch := opts.Watch != nil && *opts.Watch
	launch := opts.LaunchMode
	if launch == "" {
		launch = syncfolder.LaunchForegroundIfRunning
	}
	syncOpts := syncfolder.Options{APIURL: c.http.URL, Token: c.http.Token, HTTPClient: c.http.HTTPClient, UDID: key, BasisCacheDir: basis, Install: &install, Watch: watch, LaunchMode: launch, Compression: "identity", OnSyncComplete: opts.OnSyncComplete, Log: func(level, msg string) {
		var l slog.Level
		_ = l.UnmarshalText([]byte(level))
		c.logger.Log(ctx, l, msg)
	}}
	if err = os.MkdirAll(basis, 0755); err != nil {
		return nil, err
	}
	if err = syncfolder.BootstrapAndroidBasisCache(ctx, absolute, basis, syncOpts, opts.OnBasisDownloadProgress); err != nil {
		return nil, err
	}
	return syncfolder.Sync(ctx, absolute, syncOpts)
}
