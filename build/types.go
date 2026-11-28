// Types ported from @limrun/api 0.69.1 (319078ef).
package build

type XcodeBuildExecRequest struct {
	Command       string                             `json:"command"`
	Xcodebuild    *XcodeBuildExecRequestXcodebuild   `json:"xcodebuild,omitempty"`
	Xcodegen      *XcodeBuildExecRequestXcodegen     `json:"xcodegen,omitempty"`
	ReactNative   *XcodeBuildExecRequestReactNative  `json:"reactNative,omitempty"`
	Signing       *XcodeBuildExecRequestSigning      `json:"signing,omitempty"`
	CloudSigning  *XcodeBuildExecRequestCloudSigning `json:"cloudSigning,omitempty"`
	Testflight    *AppStoreUploadConfig              `json:"testflight,omitempty"`
	BuildSettings map[string]string                  `json:"buildSettings,omitempty"`
	Env           []string                           `json:"env,omitempty"`
	GitInit       *bool                              `json:"gitInit,omitempty"`
	// HotReload builds a Debug iOS simulator app whose Swift code later builds can replace
	// while it runs. When a later build with it changed only Swift code, the app on the
	// attached simulator shows the edit as the build ends and keeps its state; any other
	// build is installed and launched as usual. Lines starting with "Hot reload:" in the
	// build output say which happened.
	//
	// It needs an attached simulator; without one the build is a normal Debug build. Only the
	// iphonesimulator sdk and the Debug configuration are supported, and the configuration
	// defaults to Debug. React Native and Expo projects are built normally: Fast Refresh
	// already shows their JavaScript edits. Code that already ran, such as app startup, and
	// closures created before a reload keep their old behavior until the app restarts. Older
	// limbuild servers ignore it.
	HotReload          *bool                                    `json:"hotReload,omitempty"`
	LogProcessor       *string                                  `json:"logProcessor,omitempty"`
	AssetUpload        *AssetUpload                             `json:"assetUpload,omitempty"`
	SignedUploadURL    *string                                  `json:"signedUploadUrl,omitempty"`
	AssetID            *string                                  `json:"assetId,omitempty"`
	Webhook            *WebhookConfig                           `json:"webhook,omitempty"`
	AdditionalMetadata *XcodeBuildExecRequestAdditionalMetadata `json:"-"`
}

type XcodeBuildExecRequestXcodebuild struct {
	Workspace     *string  `json:"workspace,omitempty"`
	Project       *string  `json:"project,omitempty"`
	Scheme        *string  `json:"scheme,omitempty"`
	SDK           *string  `json:"sdk,omitempty"`
	Configuration *string  `json:"configuration,omitempty"`
	ArtifactName  *string  `json:"artifactName,omitempty"`
	Action        *string  `json:"action,omitempty"`
	OnlyTesting   []string `json:"onlyTesting,omitempty"`
	SkipTesting   []string `json:"skipTesting,omitempty"`
	RunTests      *bool    `json:"runTests,omitempty"`
}

type XcodeBuildExecRequestXcodegen struct {
	Spec        *string `json:"spec,omitempty"`
	Project     *string `json:"project,omitempty"`
	ProjectRoot *string `json:"projectRoot,omitempty"`
}

type XcodeBuildExecRequestReactNative struct {
	ExpoAppDir        *string `json:"expoAppDir,omitempty"`
	DevServerURL      *string `json:"devServerURL,omitempty"`
	ExpoForcePrebuild *bool   `json:"expoForcePrebuild,omitempty"`
}

type XcodeBuildExecRequestSigning struct {
	CertificateP12Base64       *string  `json:"certificateP12Base64,omitempty"`
	CertificatePassword        *string  `json:"certificatePassword,omitempty"`
	ProvisioningProfilesBase64 []string `json:"provisioningProfilesBase64,omitempty"`
}

type XcodeBuildExecRequestCloudSigning struct {
	Method              string            `json:"method"`
	TeamID              string            `json:"teamId"`
	APIKeyID            string            `json:"apiKeyId"`
	APIIssuerID         string            `json:"apiIssuerId"`
	APIPrivateKeyBase64 string            `json:"apiPrivateKeyBase64"`
	Entitlements        map[string]string `json:"entitlements,omitempty"`
}

type XcodeBuildExecRequestAdditionalMetadata struct {
	SignedDownloadURL *string `json:"signedDownloadUrl,omitempty"`
}

type WebhookConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

type GradleAndroidABI = string

type GradleReactNativeConfig struct {
	ExpoAppDir    *string            `json:"expoAppDir,omitempty"`
	Architectures []GradleAndroidABI `json:"architectures,omitempty"`
}

type GradleSigningConfig struct {
	KeystoreBase64   string `json:"keystoreBase64"`
	KeystorePassword string `json:"keystorePassword"`
	KeyAlias         string `json:"keyAlias"`
	KeyPassword      string `json:"keyPassword"`
}

type GradleBuildExecRequest struct {
	Command            string                                    `json:"command"`
	Env                []string                                  `json:"env,omitempty"`
	Tasks              []string                                  `json:"tasks,omitempty"`
	ProjectPath        *string                                   `json:"projectPath,omitempty"`
	ReactNative        *GradleReactNativeConfig                  `json:"reactNative,omitempty"`
	Signing            *GradleSigningConfig                      `json:"signing,omitempty"`
	Playstore          *GradlePlaystoreConfig                    `json:"playstore,omitempty"`
	AssetUpload        *AssetUpload                              `json:"assetUpload,omitempty"`
	SignedUploadURL    *string                                   `json:"signedUploadUrl,omitempty"`
	Webhook            *WebhookConfig                            `json:"webhook,omitempty"`
	AdditionalMetadata *GradleBuildExecRequestAdditionalMetadata `json:"-"`
}

type GradleBuildExecRequestAdditionalMetadata struct {
	SignedDownloadURL *string `json:"signedDownloadUrl,omitempty"`
}

type GradlePlaystoreConfig struct {
	AccessToken              *string `json:"accessToken,omitempty"`
	ServiceAccountJsonBase64 *string `json:"serviceAccountJsonBase64,omitempty"`
	Track                    *string `json:"track,omitempty"`
	ReleaseStatus            *string `json:"releaseStatus,omitempty"`
	PackageName              *string `json:"packageName,omitempty"`
	AutoIncrementVersionCode *bool   `json:"autoIncrementVersionCode,omitempty"`
}

type RunExecRequest struct {
	Command        string   `json:"command"`
	CommandLine    string   `json:"commandLine"`
	Cwd            *string  `json:"cwd,omitempty"`
	Env            []string `json:"env,omitempty"`
	TimeoutSeconds *float64 `json:"timeoutSeconds,omitempty"`
}

type ExecResult struct {
	ExitCode          int                   `json:"exitCode"`
	ExecID            string                `json:"execId"`
	Status            string                `json:"status"`
	SignedDownloadURL *string               `json:"signedDownloadUrl,omitempty"`
	Appstore          *AppStoreEvent        `json:"appstore,omitempty"`
	Playstore         *PlaystoreEvent       `json:"playstore,omitempty"`
	Xctest            *ExecResultXctest     `json:"xctest,omitempty"`
	TimedOut          *bool                 `json:"timedOut,omitempty"`
	Incomplete        *ExecResultIncomplete `json:"incomplete,omitempty"`
}

type ExecResultXctest struct {
	Cases   []XctestCaseEvent   `json:"cases"`
	Summary *XctestSummaryEvent `json:"summary,omitempty"`
}

type ExecResultIncomplete struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type XctestCaseEvent struct {
	Type           string   `json:"type"`
	TestClass      string   `json:"testClass"`
	Method         string   `json:"method"`
	Passed         bool     `json:"passed"`
	DurationMs     float64  `json:"durationMs"`
	FailureMessage *string  `json:"failureMessage,omitempty"`
	FailureFile    *string  `json:"failureFile,omitempty"`
	FailureLine    *float64 `json:"failureLine,omitempty"`
}

type XctestSummaryEvent struct {
	Type         string  `json:"type"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	PlanFinished bool    `json:"planFinished"`
	Error        *string `json:"error,omitempty"`
}

type AppStoreEvent struct {
	AppID    *string `json:"appId,omitempty"`
	State    string  `json:"state"`
	UploadID *string `json:"uploadId,omitempty"`
	BuildID  *string `json:"buildId,omitempty"`
}

type PlaystoreEvent struct {
	State       string   `json:"state"`
	VersionCode *float64 `json:"versionCode,omitempty"`
	Track       *string  `json:"track,omitempty"`
	Code        *string  `json:"code,omitempty"`
	Message     *string  `json:"message,omitempty"`
}

type AppStoreUploadConfig struct {
	AppID                    *string  `json:"appId,omitempty"`
	APIKeyID                 string   `json:"apiKeyId"`
	APIIssuerID              *string  `json:"apiIssuerId,omitempty"`
	APIPrivateKeyBase64      string   `json:"apiPrivateKeyBase64"`
	WaitTimeoutSeconds       *float64 `json:"waitTimeoutSeconds,omitempty"`
	AutoIncrementBuildNumber *bool    `json:"autoIncrementBuildNumber,omitempty"`
}

type ExecLogEvent struct {
	ID   *string `json:"id,omitempty"`
	Type string  `json:"type"`
	Data string  `json:"data"`
}

type ExecLogResult struct {
	ExecID   string `json:"execId"`
	Status   string `json:"status"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

type AssetUpload struct {
	Target        string               `json:"target"`
	Token         *string              `json:"token,omitempty"`
	TTL           *string              `json:"ttl,omitempty"`
	UploadOptions *AssetUploadMetadata `json:"uploadOptions,omitempty"`
}

type AssetUploadMetadata struct {
	DisplayName      *string `json:"displayName,omitempty"`
	BundleIdentifier *string `json:"bundleIdentifier,omitempty"`
	ShortVersion     *string `json:"shortVersion,omitempty"`
	BuildVersion     *string `json:"buildVersion,omitempty"`
	Deeplink         *string `json:"deeplink,omitempty"`
}
