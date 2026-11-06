package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/limrun-inc/go-sdk/internal/direct"
	"github.com/limrun-inc/go-sdk/internal/stream"
	"github.com/limrun-inc/go-sdk/syncfolder"
)

type Option func(*ExecOptions)

func WithHTTPClient(client *http.Client) Option {
	return func(o *ExecOptions) { o.HTTPClient = client }
}
func WithExecutionOptions(options ExecOptions) Option {
	return func(o *ExecOptions) { api, token := o.APIURL, o.Token; *o = options; o.APIURL = api; o.Token = token }
}

type DaemonClient struct {
	options  ExecOptions
	http     direct.Client
	platform string
}
type XcodeClient struct{ *DaemonClient }
type GradleClient struct{ *DaemonClient }

func newDaemon(apiURL, token, platform string, opts []Option) *DaemonClient {
	o := ExecOptions{APIURL: apiURL, Token: token}
	for _, f := range opts {
		f(&o)
	}
	return &DaemonClient{options: o, http: direct.Client{URL: apiURL, Token: token, HTTPClient: o.HTTPClient}, platform: platform}
}
func NewXcodeClient(apiURL, token string, opts ...Option) *XcodeClient {
	return &XcodeClient{newDaemon(apiURL, token, "xcode", opts)}
}
func NewGradleClient(apiURL, token string, opts ...Option) *GradleClient {
	return &GradleClient{newDaemon(apiURL, token, "gradle", opts)}
}

type SyncOptions struct {
	Watch, Install  *bool
	BasisCacheDir   string
	Ignore, Include func(string) bool
	AdditionalFiles []syncfolder.AdditionalFile
	OnSyncComplete  func(syncfolder.CompleteEvent)
}

func (c *DaemonClient) Sync(ctx context.Context, local string, opts *SyncOptions) (*syncfolder.Result, error) {
	if opts == nil {
		opts = &SyncOptions{}
	}
	key, cache, err := syncfolder.BasisCache(local, opts.BasisCacheDir)
	if err != nil {
		return nil, err
	}
	install, watch := c.platform == "xcode", c.platform == "xcode"
	if opts.Install != nil && c.platform == "xcode" {
		install = *opts.Install
	}
	if opts.Watch != nil {
		watch = *opts.Watch
	}
	nested := true
	additional := opts.Ignore
	if c.platform == "gradle" {
		additional = func(p string) bool {
			trimmed := strings.TrimSuffix(p, "/")
			if filepath.Base(trimmed) == "local.properties" || trimmed == ".gradle" || trimmed == ".kotlin" || trimmed == "build" {
				return true
			}
			return opts.Ignore != nil && opts.Ignore(p)
		}
	}
	ignore, err := syncfolder.CreateIgnore(local, syncfolder.IgnoreOptions{BasisCacheDir: cache, XcodeDefaults: c.platform == "xcode", NestedGitignore: &nested, Additional: additional, Include: opts.Include, Log: c.options.Log})
	if err != nil {
		return nil, err
	}
	return syncfolder.Sync(ctx, local, syncfolder.Options{APIURL: c.options.APIURL, Token: c.options.Token, HTTPClient: c.options.HTTPClient, UDID: key, BasisCacheDir: cache, Install: &install, Watch: watch, SyncSymlinks: true, LaunchMode: syncfolder.LaunchForegroundIfRunning, Ignore: ignore, AdditionalFiles: opts.AdditionalFiles, OnSyncComplete: opts.OnSyncComplete, Log: c.options.Log})
}
func (c *DaemonClient) PullFile(ctx context.Context, remote, local string) error {
	destination, err := filepath.Abs(local)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(filepath.Dir(destination), ".lim-download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	temp := filepath.Join(dir, "download")
	if err = c.http.Download(ctx, strings.TrimRight(c.options.APIURL, "/")+"/files?path="+url.QueryEscape(remote), temp); err != nil {
		return err
	}
	return os.Rename(temp, destination)
}

type RunOptions struct {
	Cwd            string
	Env            []string
	TimeoutSeconds *float64
}

func (c *DaemonClient) Run(ctx context.Context, commandLine string, opts *RunOptions) (*Process, error) {
	if strings.TrimSpace(commandLine) == "" {
		return nil, errors.New("commandLine must not be empty")
	}
	if opts == nil {
		opts = &RunOptions{}
	}
	if opts.TimeoutSeconds != nil {
		n := *opts.TimeoutSeconds
		if math.IsNaN(n) || n < 1 || n > 21600 || math.Trunc(n) != n {
			return nil, errors.New("timeoutSeconds must be an integer between 1 and 21600")
		}
	}
	cwd := opts.Cwd
	if cwd == "" {
		cwd = "."
	}
	return Exec(ctx, RunExecRequest{Command: "run", CommandLine: commandLine, Cwd: &cwd, Env: opts.Env, TimeoutSeconds: opts.TimeoutSeconds}, c.options), nil
}
func (c *DaemonClient) ObserveBuildLogs(ctx context.Context, id string, opts ExecLogOptions) (*ExecLogResult, error) {
	return ObserveExecLogs(ctx, id, c.options, opts)
}

type BuildUpload struct {
	Target          *string
	AssetName       *string
	SignedUploadURL *string
	Token           *string
	TTL             string
	UploadOptions   *AssetUploadMetadata
}

var assetIDPattern = regexp.MustCompile(`^asset_[0-7][0-9a-hjkmnp-tv-z]{25}$`)

func NormalizeAssetUpload(upload BuildUpload) (*AssetUpload, *string, error) {
	modes := 0
	for _, v := range []*string{upload.Target, upload.AssetName, upload.SignedUploadURL} {
		if v != nil {
			modes++
		}
	}
	if modes != 1 {
		return nil, nil, errors.New("provide exactly one upload target or signedUploadUrl")
	}
	if upload.SignedUploadURL != nil {
		if *upload.SignedUploadURL == "" {
			return nil, nil, errors.New("signedUploadUrl must not be empty")
		}
		return nil, upload.SignedUploadURL, nil
	}
	target := ""
	if upload.Target != nil {
		target = *upload.Target
	} else {
		if *upload.AssetName == "" {
			return nil, nil, errors.New("assetName must not be empty")
		}
		target = "name/" + *upload.AssetName
	}
	if strings.HasPrefix(target, "name/") {
		if target == "name/" {
			return nil, nil, errors.New("name targets must include an exact name after name/")
		}
	} else if !assetIDPattern.MatchString(target) {
		return nil, nil, errors.New("upload target must be an asset ID or an exact name target")
	}
	if upload.Token != nil && strings.TrimSpace(*upload.Token) == "" {
		return nil, nil, errors.New("upload token must not be empty")
	}
	ttl := upload.TTL
	if ttl == "" {
		ttl = "336h"
	}
	return &AssetUpload{Target: target, Token: upload.Token, TTL: &ttl, UploadOptions: upload.UploadOptions}, nil, nil
}

type GradleBuildOptions struct {
	GradleBuildExecRequest
	Upload *BuildUpload
}

func (c *GradleClient) Gradlebuild(ctx context.Context, opts *GradleBuildOptions) (*Process, error) {
	var req GradleBuildExecRequest
	if opts != nil {
		req = opts.GradleBuildExecRequest
		if opts.Upload != nil {
			var err error
			req.AssetUpload, req.SignedUploadURL, err = NormalizeAssetUpload(*opts.Upload)
			if err != nil {
				return nil, err
			}
		}
	}
	req.Command = "gradlebuild"
	return Exec(ctx, req, c.options), nil
}

type XcodeProjectConfig = XcodeBuildExecRequestXcodebuild
type XcodeBuildOptions struct {
	XcodeBuildExecRequest
	Appstore      *AppStoreUploadConfig
	Upload        *BuildUpload
	OnXctestEvent func(XctestEvent)
}

func (c *XcodeClient) Xcodebuild(ctx context.Context, settings *XcodeProjectConfig, opts *XcodeBuildOptions) (*Process, error) {
	var req XcodeBuildExecRequest
	options := c.options
	if opts != nil {
		req = opts.XcodeBuildExecRequest
		if opts.Appstore != nil {
			req.Testflight = opts.Appstore
		}
		if opts.OnXctestEvent != nil {
			options.OnXctestEvent = opts.OnXctestEvent
		}
		if opts.Upload != nil {
			var err error
			req.AssetUpload, req.SignedUploadURL, err = NormalizeAssetUpload(*opts.Upload)
			if err != nil {
				return nil, err
			}
		}
	}
	req.Command = "xcodebuild"
	req.Xcodebuild = settings
	if settings != nil {
		action := ""
		if settings.Action != nil {
			action = *settings.Action
		}
		if settings.Configuration != nil && *settings.Configuration == "Release" && req.ReactNative != nil && req.ReactNative.DevServerURL != nil && *req.ReactNative.DevServerURL != "" {
			return nil, errors.New("reactNative.devServerURL is only supported for Debug builds")
		}
		if len(settings.OnlyTesting) > 0 && len(settings.SkipTesting) > 0 {
			return nil, errors.New("onlyTesting and skipTesting are mutually exclusive; pass one")
		}
		if (len(settings.OnlyTesting) > 0 || len(settings.SkipTesting) > 0) && action != "build-for-testing" {
			return nil, errors.New("onlyTesting/skipTesting require action: 'build-for-testing'")
		}
		if settings.RunTests != nil && !*settings.RunTests && action != "build-for-testing" {
			return nil, errors.New("runTests: false requires action: 'build-for-testing'")
		}
		if action == "build-for-testing" && settings.SDK != nil && *settings.SDK != "iphonesimulator" {
			return nil, errors.New("action: 'build-for-testing' requires the iphonesimulator sdk")
		}
	}
	if err := ValidateBuildSettings(req.BuildSettings); err != nil {
		return nil, err
	}
	return Exec(ctx, req, options), nil
}

var BuildSettingKeyPattern = regexp.MustCompile(`^[A-Z0-9_]+$`)

func ValidateBuildSettings(settings map[string]string) error {
	if len(settings) > 32 {
		return fmt.Errorf("too many build settings: got %d, max 32", len(settings))
	}
	total := 0
	for key, value := range settings {
		if !BuildSettingKeyPattern.MatchString(key) {
			return fmt.Errorf("invalid build setting key %q: keys must match ^[A-Z0-9_]+$", key)
		}
		if len(value) > 4096 {
			return fmt.Errorf("build setting value for %q is too large: got %d bytes, max 4096 bytes", key, len(value))
		}
		total += len(key) + len(value)
		if total > 65536 {
			return errors.New("build settings payload is too large: max 65536 bytes")
		}
	}
	return nil
}
func ParseBuildSettingEntries(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	settings := map[string]string{}
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid build setting %q: expected KEY=VALUE", entry)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid build setting %q: key is required", entry)
		}
		if _, exists := settings[key]; exists {
			return nil, fmt.Errorf("duplicate build setting key %q", key)
		}
		settings[key] = value
	}
	if err := ValidateBuildSettings(settings); err != nil {
		return nil, err
	}
	return settings, nil
}
func ParseBuildSettings(entries []string) (map[string]string, error) {
	return ParseBuildSettingEntries(entries)
}

type RbeUnsupportedError struct{ Operation string }

func (e *RbeUnsupportedError) Error() string {
	return e.Operation + " returned 404: this instance may predate RBE support; recreate the instance"
}

type XcodeSelectionUnsupportedError struct{ Operation string }

func (e *XcodeSelectionUnsupportedError) Error() string {
	return e.Operation + " returned 404: this instance may predate Xcode selection support; recreate the instance"
}
func (c *XcodeClient) request(ctx context.Context, method, path string, params, out any) error {
	err := c.http.JSON(ctx, method, path, params, out)
	var he *direct.HTTPError
	if errors.As(err, &he) && he.Status == 404 {
		if path == "xcode" {
			return &XcodeSelectionUnsupportedError{method + " /" + path}
		}
		if path == "rbe" || strings.HasPrefix(path, "rbe/builds/") {
			return &RbeUnsupportedError{method + " /" + path}
		}
	}
	return err
}
func (c *XcodeClient) GetSimulator(ctx context.Context) (*SimulatorStatus, error) {
	var r SimulatorStatus
	err := c.request(ctx, http.MethodGet, "simulator", nil, &r)
	return &r, err
}
func (c *XcodeClient) AttachSimulator(ctx context.Context, apiURL, token string) (*SimulatorAttachResult, error) {
	var r SimulatorAttachResult
	err := c.request(ctx, http.MethodPost, "simulator", map[string]any{"apiUrl": apiURL, "token": token}, &r)
	return &r, err
}
func (c *XcodeClient) GetXcode(ctx context.Context) (*XcodeStatus, error) {
	var r XcodeStatus
	err := c.request(ctx, http.MethodGet, "xcode", nil, &r)
	return &r, err
}
func (c *XcodeClient) SetXcode(ctx context.Context, version string) (*XcodeSelectResult, error) {
	var r XcodeSelectResult
	err := c.request(ctx, http.MethodPost, "xcode", map[string]string{"version": version}, &r)
	return &r, err
}
func (c *XcodeClient) GetInfo(ctx context.Context) (*SandboxInfo, error) {
	var r SandboxInfo
	if err := c.request(ctx, http.MethodGet, "info", nil, &r); err != nil {
		return nil, err
	}
	if r.HomeDir == "" {
		return nil, errors.New("GET /info returned no homeDir")
	}
	if strings.HasPrefix(r.HomeDir, "/") || strings.ContainsAny(r.HomeDir, "\\\x00") {
		return nil, fmt.Errorf("invalid sandbox home path from server: %s", r.HomeDir)
	}
	for _, part := range strings.Split(r.HomeDir, "/") {
		if part == "" || part == "." || part == ".." {
			return nil, fmt.Errorf("invalid sandbox home path from server: %s", r.HomeDir)
		}
	}
	return &r, nil
}
func (c *XcodeClient) StartRbe(ctx context.Context, opts *RbeStartOptions) (*RbeStatus, error) {
	if opts == nil {
		opts = &RbeStartOptions{}
	}
	var r RbeStatus
	err := c.request(ctx, http.MethodPost, "rbe", opts, &r)
	return &r, err
}
func (c *XcodeClient) GetRbe(ctx context.Context) (*RbeStatus, error) {
	var r RbeStatus
	err := c.request(ctx, http.MethodGet, "rbe", nil, &r)
	return &r, err
}
func (c *XcodeClient) StopRbe(ctx context.Context) (*RbeStatus, error) {
	var r RbeStatus
	err := c.request(ctx, http.MethodDelete, "rbe", nil, &r)
	return &r, err
}
func (c *XcodeClient) GetActiveRbeBuilds(ctx context.Context) ([]RbeActiveBuild, error) {
	var r []RbeActiveBuild
	err := c.request(ctx, http.MethodGet, "rbe/builds/active", nil, &r)
	return r, err
}
func (c *XcodeClient) GetRecentRbeBuilds(ctx context.Context) ([]RbeBuildSummary, error) {
	var r []RbeBuildSummary
	err := c.request(ctx, http.MethodGet, "rbe/builds/recent", nil, &r)
	return r, err
}
func (c *XcodeClient) WaitForRbeBuildEnd(ctx context.Context, id string) (*RbeBuildEnd, error) {
	res, err := c.http.Do(ctx, http.MethodGet, strings.TrimRight(c.http.URL, "/")+"/exec/"+url.PathEscape(id)+"/events", nil, "", -1)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var result RbeBuildEnd
	err = stream.Read(res.Body, func(e stream.Event) error {
		if e.Type != "end" {
			return nil
		}
		if err := json.Unmarshal([]byte(e.Data), &result); err != nil {
			return fmt.Errorf("invalid build end event for %s: %w", id, err)
		}
		return errTerminal
	}, func() {}, func(time.Duration) {})
	if errors.Is(err, errTerminal) {
		return &result, nil
	}
	return nil, fmt.Errorf("build event stream for %s ended without a terminal event: %w", id, err)
}
