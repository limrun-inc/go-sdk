// Package android controls Limrun Android instances over their HTTP and WebSocket APIs.
package android

import (
	"time"

	"github.com/limrun-inc/go-sdk/session"
	"github.com/limrun-inc/go-sdk/websocket/ios"
)

type Selector struct {
	ResourceID     string  `json:"resourceId,omitempty"`
	Text           *string `json:"text,omitempty"`
	ContentDesc    *string `json:"contentDesc,omitempty"`
	ClassName      string  `json:"className,omitempty"`
	PackageName    string  `json:"packageName,omitempty"`
	Index          *int    `json:"index,omitempty"`
	Clickable      *bool   `json:"clickable,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
	Focused        *bool   `json:"focused,omitempty"`
	BoundsContains *Point  `json:"boundsContains,omitempty"`
}
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type ElementTarget struct {
	Selector *Selector `json:"selector,omitempty"`
	X        *float64  `json:"x,omitempty"`
	Y        *float64  `json:"y,omitempty"`
}

func At(x, y float64) ElementTarget { return ElementTarget{X: &x, Y: &y} }

type Bounds struct {
	Left    float64 `json:"left"`
	Top     float64 `json:"top"`
	Right   float64 `json:"right"`
	Bottom  float64 `json:"bottom"`
	CenterX float64 `json:"centerX"`
	CenterY float64 `json:"centerY"`
}
type ElementNode struct {
	Index        string  `json:"index,omitempty"`
	Text         string  `json:"text,omitempty"`
	ResourceID   string  `json:"resourceId,omitempty"`
	ClassName    string  `json:"className,omitempty"`
	PackageName  string  `json:"packageName,omitempty"`
	ContentDesc  string  `json:"contentDesc,omitempty"`
	Clickable    *bool   `json:"clickable,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
	Focusable    *bool   `json:"focusable,omitempty"`
	Focused      *bool   `json:"focused,omitempty"`
	Scrollable   *bool   `json:"scrollable,omitempty"`
	Selected     *bool   `json:"selected,omitempty"`
	Bounds       string  `json:"bounds,omitempty"`
	ParsedBounds *Bounds `json:"parsedBounds,omitempty"`
}
type ScreenshotData struct {
	DataURI string `json:"dataUri"`
}
type ElementTreeData struct {
	XML   string        `json:"xml"`
	Nodes []ElementNode `json:"nodes"`
}
type ElementTreeOptions struct{ WaitForIdleTimeout time.Duration }
type FindElementResult struct {
	Elements []ElementNode `json:"elements"`
	Count    int           `json:"count"`
}
type TapResult = Point
type SetTextResult struct {
	TextLength int `json:"textLength"`
}
type PressKeyResult struct {
	Key string `json:"key"`
}
type ScrollDirection string

const (
	ScrollUp    ScrollDirection = "up"
	ScrollDown  ScrollDirection = "down"
	ScrollLeft  ScrollDirection = "left"
	ScrollRight ScrollDirection = "right"
)

type ScrollResult struct {
	Direction ScrollDirection `json:"direction"`
	StartX    float64         `json:"startX"`
	StartY    float64         `json:"startY"`
	EndX      float64         `json:"endX"`
	EndY      float64         `json:"endY"`
}
type OpenURLResult struct {
	URL string `json:"url"`
}
type LaunchAppMode string

const (
	ForegroundIfRunning LaunchAppMode = "ForegroundIfRunning"
	RelaunchIfRunning   LaunchAppMode = "RelaunchIfRunning"
)

type AppExitReason string

const (
	ExitCrash      AppExitReason = "crash"
	ExitANR        AppExitReason = "anr"
	ExitNormal     AppExitReason = "exit"
	ExitTerminated AppExitReason = "terminated"
)

type AppCrashInfo = ios.AppCrashInfo
type AppANRInfo struct {
	ProcessName  string `json:"processName"`
	PID          int    `json:"pid"`
	ProcessStats string `json:"processStats"`
}
type AppExitInfo struct {
	PackageName string        `json:"packageName"`
	Reason      AppExitReason `json:"reason"`
	Crash       *AppCrashInfo `json:"crash,omitempty"`
	ANR         *AppANRInfo   `json:"anr,omitempty"`
}
type LaunchAppExitCallback func([]string, AppExitInfo)
type LaunchAppOptions struct {
	Mode   LaunchAppMode
	OnExit LaunchAppExitCallback
}
type LaunchAppResult struct {
	PackageName string `json:"packageName"`
}
type InstalledApp = ios.InstalledApp
type PlayOnMicrophoneResult struct {
	Duration   float64 `json:"duration"`
	Once       bool    `json:"once"`
	Generation int64   `json:"generation"`
}
type WifiBandwidthOptions struct {
	DownKbps *int64 `json:"downKbps,omitempty"`
	UpKbps   *int64 `json:"upKbps,omitempty"`
}
type ADBShellOptions struct {
	Timeout time.Duration
	// TimeoutMS overrides Timeout and can explicitly request zero.
	TimeoutMS *int64
	// Encoding accepts utf8, utf16le, latin1, ascii, hex, base64 and base64url.
	Encoding string
}
type ADBShellResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Truncated bool
}
type PersistOption = session.PersistOption
type RecordingOptions struct {
	Quality     int
	Persist     *PersistOption
	RecordingID string
}
type RecordingDestination struct {
	PresignedURL string
	LocalPath    string
}
