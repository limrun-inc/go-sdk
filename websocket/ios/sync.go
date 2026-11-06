package ios

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/limrun-inc/go-sdk/syncfolder"
)

type SyncAppOptions struct {
	Install, Watch *bool
	BasisCacheDir  string
	LaunchMode     syncfolder.LaunchMode
	OnSyncComplete func(syncfolder.CompleteEvent)
}

func (c *Client) SyncApp(ctx context.Context, localPath string, options ...SyncAppOptions) (*syncfolder.Result, error) {
	var opts SyncAppOptions
	if len(options) > 0 {
		opts = options[0]
	}
	prepared, err := syncfolder.PrepareAppBundlePath(localPath)
	if err != nil {
		return nil, err
	}
	plist := filepath.Join(prepared.AppPath, "Info.plist")
	stat, err := os.Stat(plist)
	if err != nil || !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("the folder is not a valid app bundle: missing Info.plist at %s", plist)
	}
	key, basis, err := syncfolder.BasisCache(prepared.CacheIdentityPath, opts.BasisCacheDir)
	if err != nil {
		return nil, err
	}
	log := func(level, msg string) {
		var l slog.Level
		_ = l.UnmarshalText([]byte(level))
		c.logger.Log(ctx, l, msg)
	}
	ignore, err := syncfolder.CreateIgnore(prepared.AppPath, syncfolder.IgnoreOptions{BasisCacheDir: basis, Log: log})
	if err != nil {
		return nil, err
	}
	install := true
	if opts.Install != nil {
		install = *opts.Install
	}
	watch := true
	if opts.Watch != nil {
		watch = *opts.Watch
	}
	launch := opts.LaunchMode
	if launch == "" {
		launch = syncfolder.LaunchForegroundIfRunning
	}
	syncOpts := syncfolder.Options{APIURL: c.apiURL, Token: c.token, HTTPClient: c.httpClient, UDID: key, BasisCacheDir: basis, Install: &install, LaunchMode: launch, Watch: watch && !prepared.IsArchive, Ignore: ignore, Log: log, OnSyncComplete: opts.OnSyncComplete}
	result, err := syncfolder.Sync(ctx, prepared.AppPath, syncOpts)
	if err != nil {
		return nil, err
	}
	if prepared.IsArchive && watch {
		watcher, e := syncfolder.WatchAppArchive(ctx, prepared.ArchivePath, syncfolder.ArchiveWatchOptions{Log: log, OnExtracted: func() error { _, e := syncfolder.Sync(ctx, prepared.AppPath, syncOpts); return e }})
		if e != nil {
			return nil, e
		}
		result.StopWatching = watcher.Close
	}
	return result, nil
}
