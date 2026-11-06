package ios

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/limrun-inc/go-sdk/internal/control"
)

type ConnectionState = control.State

const (
	ConnectionConnecting   = control.Connecting
	ConnectionConnected    = control.Connected
	ConnectionDisconnected = control.Disconnected
	ConnectionReconnecting = control.Reconnecting
)

func WithReconnect(maxAttempts int, delay, maxDelay time.Duration) Option {
	return func(c *Client) {
		c.reconnectAttempts = maxAttempts
		c.reconnectDelay = delay
		c.maxReconnectDelay = maxDelay
	}
}
func (c *Client) GetConnectionState() ConnectionState {
	if c.transport == nil {
		return ConnectionDisconnected
	}
	return c.transport.State()
}
func (c *Client) OnConnectionStateChange(fn func(ConnectionState)) func() {
	return c.transport.OnStateChange(fn)
}

type AppExitReason string

const (
	AppExitCrash      AppExitReason = "crash"
	AppExitNormal     AppExitReason = "exit"
	AppExitTerminated AppExitReason = "terminated"
)

type AppCrashInfo struct {
	ProcessName string  `json:"processName"`
	PID         int     `json:"pid"`
	ShortMsg    string  `json:"shortMsg"`
	LongMsg     string  `json:"longMsg"`
	StackTrace  string  `json:"stackTrace"`
	TimeMillis  float64 `json:"timeMillis"`
}
type AppExitInfo struct {
	BundleID string        `json:"bundleId"`
	Reason   AppExitReason `json:"reason"`
	Crash    *AppCrashInfo `json:"crash,omitempty"`
}
type AppHangInfo struct {
	BundleID           string   `json:"bundleId"`
	PID                int      `json:"pid"`
	HangID             int      `json:"hangId"`
	Ended              bool     `json:"ended"`
	DurationMs         float64  `json:"durationMs"`
	MainThreadState    *string  `json:"mainThreadState,omitempty"`
	MainThreadCPUMs    *float64 `json:"mainThreadCpuMs,omitempty"`
	MainThreadKernelMs *float64 `json:"mainThreadKernelMs,omitempty"`
	AppCPUMs           *float64 `json:"appCpuMs,omitempty"`
	PageIns            *float64 `json:"pageIns,omitempty"`
	FootprintBytes     *float64 `json:"footprintBytes,omitempty"`
	StackTrace         *string  `json:"stackTrace,omitempty"`
}
type LaunchAppExitCallback func([]string, AppExitInfo)
type AppHangCallback func(AppHangInfo)
type AppMonitorCallbacks struct {
	OnExit LaunchAppExitCallback
	OnHang AppHangCallback
}
type AppWatch struct {
	ExecID string
	client *Client
}

func (w *AppWatch) Stop(ctx context.Context) error {
	w.client.notifications.Delete(w.ExecID)
	return w.client.call(ctx, "unwatchApp", map[string]any{"execId": w.ExecID}, nil, 0)
}
func WithLaunchEnvironment(env map[string]string) LaunchAppOption {
	return func(c *launchAppConfig) { c.env = env }
}
func WithAppExit(fn LaunchAppExitCallback) LaunchAppOption {
	return func(c *launchAppConfig) { c.onExit = fn }
}
func WithAppHang(fn AppHangCallback) LaunchAppOption {
	return func(c *launchAppConfig) { c.onHang = fn }
}

type appNotification struct {
	mu         sync.Mutex
	queue      []json.RawMessage
	running    bool
	callbacks  AppMonitorCallbacks
	persistent bool
	exited     bool
}

func (c *Client) registerNotification(id string, callbacks AppMonitorCallbacks, persistent bool) {
	c.notifications.Store(id, &appNotification{callbacks: callbacks, persistent: persistent})
}
func (c *Client) handleNotification(data json.RawMessage) {
	var head struct {
		Type   string `json:"type"`
		ExecID string `json:"execId"`
	}
	if json.Unmarshal(data, &head) != nil {
		return
	}
	v, ok := c.notifications.Load(head.ExecID)
	if !ok {
		return
	}
	n := v.(*appNotification)
	n.mu.Lock()
	if n.exited {
		n.mu.Unlock()
		return
	}
	if head.Type == "appExit" && !n.persistent {
		n.exited = true
	}
	n.queue = append(n.queue, append(json.RawMessage(nil), data...))
	if n.running {
		n.mu.Unlock()
		return
	}
	n.running = true
	n.mu.Unlock()
	go func() {
		for {
			n.mu.Lock()
			if len(n.queue) == 0 {
				n.running = false
				n.mu.Unlock()
				return
			}
			item := n.queue[0]
			n.queue = n.queue[1:]
			n.mu.Unlock()
			if current, ok := c.notifications.Load(head.ExecID); !ok || current != n {
				continue
			}
			c.deliverNotification(head.ExecID, n, item)
		}
	}()
}
func (c *Client) deliverNotification(id string, n *appNotification, data json.RawMessage) {
	defer func() {
		if value := recover(); value != nil {
			c.logger.Error("app callback panicked", "panic", value)
		}
	}()
	var msg struct {
		Type string `json:"type"`
		AppExitInfo
		Logs         json.RawMessage `json:"logs"`
		LogLineCount int             `json:"logLineCount"`
		Hang         AppHangInfo     `json:"hang"`
	}
	if json.Unmarshal(data, &msg) != nil {
		return
	}
	if msg.Type == "appHang" {
		if n.callbacks.OnHang != nil {
			msg.Hang.BundleID = msg.BundleID
			n.callbacks.OnHang(msg.Hang)
		}
		return
	}
	if !n.persistent {
		defer c.notifications.Delete(id)
	}
	if n.callbacks.OnExit == nil {
		return
	}
	logs := []string{}
	if len(msg.Logs) > 0 && msg.Logs[0] == '[' {
		if err := json.Unmarshal(msg.Logs, &logs); err != nil {
			c.logger.Error("decode exit logs", "error", err)
		}
	} else if msg.LogLineCount > 0 {
		raw, err := c.AppLogTail(context.Background(), msg.BundleID, msg.LogLineCount)
		if err != nil {
			c.logger.Error("fetch exit logs", "error", err)
		} else {
			raw = strings.ReplaceAll(raw, "\r\n", "\n")
			logs = strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
			if raw == "" {
				logs = []string{}
			}
		}
	}
	if msg.Reason == "" {
		msg.Reason = AppExitNormal
	}
	n.callbacks.OnExit(logs, msg.AppExitInfo)
}

// WatchApp observes one exit. Use MonitorApp to keep observing across launches.
func (c *Client) WatchApp(ctx context.Context, bundleID string, onExit LaunchAppExitCallback) (*AppWatch, error) {
	return c.watch(ctx, "watchApp", bundleID, AppMonitorCallbacks{OnExit: onExit}, false, 30*time.Second)
}
func (c *Client) MonitorApp(ctx context.Context, bundleID string, callbacks AppMonitorCallbacks) (*AppWatch, error) {
	return c.watch(ctx, "monitorApp", bundleID, callbacks, true, 10*time.Second)
}
func (c *Client) watch(ctx context.Context, kind, bundleID string, callbacks AppMonitorCallbacks, persistent bool, timeout time.Duration) (*AppWatch, error) {
	id := newSessionID()
	c.registerNotification(id, callbacks, persistent)
	err := c.call(ctx, kind, map[string]any{"bundleId": bundleID, "execId": id}, nil, timeout)
	if err != nil {
		c.notifications.Delete(id)
		return nil, err
	}
	return &AppWatch{ExecID: id, client: c}, nil
}
