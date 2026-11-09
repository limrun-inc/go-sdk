// Types ported from @limrun/api 0.69.1 (319078ef).
package build

type SimulatorInstallState = string

type SimulatorDeviceInfo struct {
	Arch         string  `json:"arch"`
	Model        string  `json:"model"`
	Name         string  `json:"name"`
	OsVersion    string  `json:"osVersion"`
	Platform     string  `json:"platform"`
	ScreenHeight float64 `json:"screenHeight"`
	ScreenWidth  float64 `json:"screenWidth"`
	UDID         string  `json:"udid"`
}

type SimulatorAttachment struct {
	APIURL        string               `json:"apiUrl"`
	IosInstanceID *string              `json:"iosInstanceId,omitempty"`
	Target        *SimulatorDeviceInfo `json:"target,omitempty"`
}

type SimulatorBuildStatus struct {
	BuildID           string                `json:"buildId"`
	SDK               string                `json:"sdk"`
	BundleID          *string               `json:"bundleId,omitempty"`
	InstallState      SimulatorInstallState `json:"installState"`
	SyncDurationMs    *float64              `json:"syncDurationMs,omitempty"`
	InstallDurationMs *float64              `json:"installDurationMs,omitempty"`
}

type SimulatorStatus struct {
	Attached    bool                  `json:"attached"`
	Simulator   *SimulatorAttachment  `json:"simulator,omitempty"`
	LatestBuild *SimulatorBuildStatus `json:"latestBuild,omitempty"`
}

type SimulatorAttachResult struct {
	Attached           bool                  `json:"attached"`
	AlreadyAttached    bool                  `json:"alreadyAttached"`
	InstalledLastBuild bool                  `json:"installedLastBuild"`
	Simulator          *SimulatorAttachment  `json:"simulator,omitempty"`
	LatestBuild        *SimulatorBuildStatus `json:"latestBuild,omitempty"`
	InstallError       *string               `json:"installError,omitempty"`
}

type RbeStatus struct {
	State        string   `json:"state"`
	FrontendPort *float64 `json:"frontendPort,omitempty"`
	XcodeVersion *string  `json:"xcodeVersion,omitempty"`
	Error        *string  `json:"error,omitempty"`
}

type XcodeInfo struct {
	Major        string  `json:"major"`
	Version      string  `json:"version"`
	Channel      *string `json:"channel,omitempty"`
	Build        string  `json:"build"`
	VersionKey   string  `json:"versionKey"`
	DeveloperDir string  `json:"developerDir"`
	BetaSeed     *string `json:"betaSeed,omitempty"`
	NodeDefault  bool    `json:"nodeDefault"`
}

type XcodeStatus struct {
	Bound     XcodeInfo   `json:"bound"`
	Installed []XcodeInfo `json:"installed"`
}

type XcodeSelectResult struct {
	Bound            XcodeInfo `json:"bound"`
	AlreadyBound     bool      `json:"alreadyBound"`
	DerivedDataReset bool      `json:"derivedDataReset"`
}

type SandboxInfo struct {
	HomeDir string     `json:"homeDir"`
	Xcode   *XcodeInfo `json:"xcode,omitempty"`
}

type RbeStartOptions struct {
	CasMaxBytes       *float64 `json:"casMaxBytes,omitempty"`
	AcMaxBytes        *float64 `json:"acMaxBytes,omitempty"`
	WorkerConcurrency *float64 `json:"workerConcurrency,omitempty"`
}

type RbeInstallResult struct {
	Installed         bool     `json:"installed"`
	BundleID          *string  `json:"bundleId,omitempty"`
	AppName           *string  `json:"appName,omitempty"`
	SyncDurationMs    *float64 `json:"syncDurationMs,omitempty"`
	InstallDurationMs *float64 `json:"installDurationMs,omitempty"`
}

type RbeUploadResult struct {
	AppName           string  `json:"appName"`
	BundleID          *string `json:"bundleId,omitempty"`
	SignedDownloadURL *string `json:"signedDownloadUrl,omitempty"`
}

type RbeActiveBuild struct {
	InvocationID string   `json:"invocationId"`
	Status       string   `json:"status"`
	Pattern      []string `json:"pattern,omitempty"`
}

type RbeBuildEnd struct {
	InvocationID string  `json:"invocationId"`
	Status       string  `json:"status"`
	Error        *string `json:"error,omitempty"`
}

type RbeBuildSummary struct {
	InvocationID string   `json:"invocationId"`
	Status       string   `json:"status"`
	Pattern      []string `json:"pattern,omitempty"`
	Error        *string  `json:"error,omitempty"`
}

type BazelBuildLog struct {
	ID                  string   `json:"id"`
	Status              string   `json:"status"`
	Pattern             []string `json:"pattern,omitempty"`
	StartedAt           *string  `json:"startedAt,omitempty"`
	DurationMs          *float64 `json:"durationMs,omitempty"`
	Error               *string  `json:"error,omitempty"`
	DownloadURL         string   `json:"downloadUrl"`
	MetadataDownloadURL string   `json:"metadataDownloadUrl"`
}

type BuildLog struct {
	ID              string   `json:"id"`
	Status          string   `json:"status"`
	ExitCode        *float64 `json:"exitCode,omitempty"`
	StartedAt       *string  `json:"startedAt,omitempty"`
	FinishedAt      *string  `json:"finishedAt,omitempty"`
	BuildDurationMs *float64 `json:"buildDurationMs,omitempty"`
	Error           *string  `json:"error,omitempty"`
	DownloadURL     string   `json:"downloadUrl"`
}
