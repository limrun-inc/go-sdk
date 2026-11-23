package ios

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/limrun-inc/go-sdk/syncfolder"
)

// The same executable the TypeScript SDK writes for its Argent bridge.
//
//go:embed shim/argent-xcrun.js
var argentXcrunSource string

// ArgentProviderCapabilities are the mechanisms the provider descriptor grants
// Argent on the simulator. Each one is served by the bridge instead of a
// process on a local Mac.
var ArgentProviderCapabilities = []string{"simulator-server", "simctl", "ax-service"}

// ArgentBridgeOptions configures StartArgentBridge.
type ArgentBridgeOptions struct {
	// ProvidersDir is the directory Argent scans for provider descriptors.
	// Defaults to ~/.argent/providers.
	ProvidersDir string
}

// ArgentBridge offers one simulator to a locally installed Argent.
type ArgentBridge struct {
	// DeviceID is the id Argent lists the simulator under; pass it as --udid
	// to Argent tools.
	DeviceID string
	// DescriptorPath is the provider descriptor Argent discovers the simulator from.
	DescriptorPath string
	// Env holds environment overrides for the Argent tool-server process.
	// Merge them over the current environment.
	Env map[string]string

	server  *http.Server
	cancel  context.CancelFunc
	workDir string
	once    sync.Once
}

// Close stops the bridge and withdraws the simulator from Argent.
func (b *ArgentBridge) Close() error {
	var err error
	b.once.Do(func() {
		if b.DescriptorPath != "" {
			_ = os.Remove(b.DescriptorPath)
		}
		b.cancel()
		err = b.server.Close()
		if e := os.RemoveAll(b.workDir); err == nil {
			err = e
		}
	})
	return err
}

// StartArgentBridge offers the simulator behind client to a locally installed
// Argent through its device-provider contract.
//
// Argent expects a simulator on the same Mac: it runs `xcrun simctl`,
// attaches to a local simulator-server for input and screenshots, and spawns
// an accessibility daemon inside the simulator. The bridge stands in for all
// three, so an unmodified Argent drives the remote simulator:
//
//   - a provider descriptor points Argent at a local HTTP and WebSocket server
//     that speaks the simulator-server protocol,
//   - an `xcrun` executable for PATH forwards simctl calls for this simulator,
//   - the same executable answers the accessibility daemon's requests when
//     Argent spawns it.
//
// The Argent tool-server must start with the returned Env, because it
// resolves `xcrun` from its own PATH. The shim needs Node.js, which Argent
// already requires.
func StartArgentBridge(ctx context.Context, client *Client, options ...ArgentBridgeOptions) (*ArgentBridge, error) {
	var opts ArgentBridgeOptions
	if len(options) > 0 {
		opts = options[0]
	}
	info, err := client.DeviceInfo(ctx)
	if err != nil {
		return nil, err
	}
	udid := info.UDID
	providerID := "limrun-" + strings.ToLower(udid[:min(8, len(udid))])

	workDir, err := os.MkdirTemp("", "limrun-argent-")
	if err != nil {
		return nil, err
	}
	mediaDir := filepath.Join(workDir, "media")
	shimDir := filepath.Join(workDir, "bin")
	for _, dir := range []string{mediaDir, shimDir} {
		if err = os.Mkdir(dir, 0o755); err != nil {
			os.RemoveAll(workDir)
			return nil, err
		}
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.RemoveAll(workDir)
		return nil, err
	}
	baseURL := "http://" + listener.Addr().String()
	bridgeCtx, cancel := context.WithCancel(context.Background())
	session := newArgentSession(bridgeCtx, client, info, mediaDir, baseURL)
	server := &http.Server{Handler: session}
	go func() { _ = server.Serve(listener) }()

	bridge := &ArgentBridge{server: server, cancel: cancel, workDir: workDir}
	fail := func(err error) (*ArgentBridge, error) {
		bridge.Close()
		return nil, err
	}

	device, _ := json.Marshal(simctlDeviceFor(info))
	bridgeURL, _ := json.Marshal(baseURL)
	source := strings.Replace(argentXcrunSource, "const bridgeUrl = process.env.LIMRUN_ARGENT_BRIDGE_URL;", "const bridgeUrl = "+string(bridgeURL)+";", 1)
	source = strings.Replace(source, "const device = JSON.parse(process.env.LIMRUN_ARGENT_DEVICE || 'null');", "const device = "+string(device)+";", 1)
	if err = os.WriteFile(filepath.Join(shimDir, "xcrun"), []byte(source), 0o755); err != nil {
		return fail(err)
	}

	providersDir := opts.ProvidersDir
	if providersDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fail(err)
		}
		providersDir = filepath.Join(home, ".argent", "providers")
	}
	if err = os.MkdirAll(providersDir, 0o755); err != nil {
		return fail(err)
	}
	descriptor, _ := json.MarshalIndent(map[string]any{
		"schemaVersion": 1,
		"id":            providerID,
		"name":          "Limrun",
		// Lets `argent providers prune` drop the descriptor if this process dies.
		"pid":        os.Getpid(),
		"supportUrl": "https://docs.limrun.com",
		"devices": []map[string]any{{
			"nativeId":     udid,
			"platform":     "ios",
			"kind":         "simulator",
			"name":         info.Model,
			"state":        "Booted",
			"capabilities": ArgentProviderCapabilities,
			// The contract requires streamUrl. The bridge serves no video stream.
			"simulatorServer": map[string]string{"apiUrl": baseURL, "streamUrl": baseURL + "/stream.mjpeg"},
		}},
	}, "", "  ")
	descriptorPath := filepath.Join(providersDir, providerID+".json")
	if err = os.WriteFile(descriptorPath, descriptor, 0o644); err != nil {
		return fail(err)
	}

	bridge.DeviceID = "ext:" + providerID + ":" + udid
	bridge.DescriptorPath = descriptorPath
	bridge.Env = map[string]string{"PATH": shimDir + string(os.PathListSeparator) + os.Getenv("PATH")}
	return bridge, nil
}

// mediaFilesKept bounds the screenshots on disk; Argent reads each one right after the call.
const mediaFilesKept = 32

type argentSession struct {
	ctx      context.Context
	client   *Client
	mediaDir string
	baseURL  string
	upgrader websocket.Upgrader

	mu         sync.Mutex
	info       *DeviceInfo
	gesture    []PerformAction
	lastEvent  time.Time
	mediaFiles []string
	input      *inputQueue
}

func newArgentSession(ctx context.Context, client *Client, info *DeviceInfo, mediaDir, baseURL string) *argentSession {
	s := &argentSession{ctx: ctx, client: client, info: info, mediaDir: mediaDir, baseURL: baseURL}
	s.input = newInputQueue(func(actions []PerformAction) error {
		_, err := client.PerformActions(ctx, actions)
		return err
	})
	return s
}

// inputQueue runs actions in arrival order. Actions that arrive while a batch
// is on the wire go out together in the next one, so a typed word costs a few
// round trips instead of two per character.
type inputQueue struct {
	run         func([]PerformAction) error
	mu          sync.Mutex
	idle        *sync.Cond
	items       []queuedInput
	busy        bool
	deferredErr error
}

type queuedInput struct {
	actions []PerformAction
	// done is nil for input acknowledged before it ran; its error is reported
	// on the next read instead.
	done chan error
}

func newInputQueue(run func([]PerformAction) error) *inputQueue {
	q := &inputQueue{run: run}
	q.idle = sync.NewCond(&q.mu)
	return q
}

func (q *inputQueue) enqueue(actions []PerformAction, acknowledged bool) <-chan error {
	item := queuedInput{actions: actions}
	if acknowledged {
		item.done = make(chan error, 1)
	}
	q.mu.Lock()
	q.items = append(q.items, item)
	if !q.busy {
		q.busy = true
		go q.drain()
	}
	q.mu.Unlock()
	return item.done
}

func (q *inputQueue) drain() {
	for {
		q.mu.Lock()
		batch := q.items
		q.items = nil
		if len(batch) == 0 {
			q.busy = false
			q.idle.Broadcast()
			q.mu.Unlock()
			return
		}
		q.mu.Unlock()
		var actions []PerformAction
		for _, item := range batch {
			actions = append(actions, item.actions...)
		}
		err := q.run(actions)
		for _, item := range batch {
			if item.done != nil {
				item.done <- err
			} else if err != nil {
				q.mu.Lock()
				q.deferredErr = err
				q.mu.Unlock()
			}
		}
	}
}

// settled waits for pending input, so a screenshot never overtakes a tap, and
// returns the failure of input that was acknowledged early.
func (q *inputQueue) settled() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.busy {
		q.idle.Wait()
	}
	err := q.deferredErr
	q.deferredErr = nil
	return err
}

func (s *argentSession) screen() (float64, float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.info.DisplayWidth > 0 && s.info.DisplayHeight > 0 {
		return s.info.DisplayWidth, s.info.DisplayHeight
	}
	return s.info.ScreenWidth, s.info.ScreenHeight
}

func (s *argentSession) orientation() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.info.Orientation != "" {
		return s.info.Orientation
	}
	return "portrait"
}

func (s *argentSession) refreshInfo(ctx context.Context) error {
	info, err := s.client.DeviceInfo(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.info = info
	s.mu.Unlock()
	return nil
}

// ---- simulator-server WebSocket -------------------------------------------

type simCommand struct {
	ID        json.RawMessage `json:"id"`
	Cmd       string          `json:"cmd"`
	Type      string          `json:"type"`
	X         float64         `json:"x"`
	Y         float64         `json:"y"`
	SecondX   *float64        `json:"second_x"`
	SecondY   *float64        `json:"second_y"`
	Code      int             `json:"code"`
	Direction string          `json:"direction"`
	Button    string          `json:"button"`
}

type commandReply struct {
	ID      any    `json:"id,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func (s *argentSession) handleCommand(raw []byte) commandReply {
	var cmd simCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		return commandReply{Status: "error", Message: "parse error: invalid JSON"}
	}
	if err := s.runCommand(cmd); err != nil {
		// simulator-server omits the id on a rejected command, and Argent matches on that.
		return commandReply{Status: "error", Message: err.Error()}
	}
	var id any
	_ = json.Unmarshal(cmd.ID, &id)
	return commandReply{ID: fmt.Sprint(id), Status: "ok"}
}

// argentButtons maps simulator-server button names to Limrun's.
var argentButtons = map[string]string{"home": "home", "power": "lock", "volumeUp": "volumeUp", "volumeDown": "volumeDown"}

// runCommand handles one simulator-server command. Argent sends coordinates
// as 0..1 fractions.
func (s *argentSession) runCommand(cmd simCommand) error {
	pressed := cmd.Direction == "Down"
	switch cmd.Cmd {
	case "touch", "edgeTouch":
		now := time.Now()
		s.mu.Lock()
		if cmd.Type == "Down" {
			s.gesture = []PerformAction{s.touchLocked("touchDown", cmd)}
			s.lastEvent = now
			s.mu.Unlock()
			return nil
		}
		if s.gesture == nil {
			s.mu.Unlock()
			return fmt.Errorf("touch %s arrived without a Down", cmd.Type)
		}
		// Argent paces a gesture by when it sends each event; replay that pace.
		if waited := now.Sub(s.lastEvent).Milliseconds(); waited > 0 {
			s.gesture = append(s.gesture, ActionWait(int(waited)))
		}
		s.lastEvent = now
		if cmd.Type == "Move" {
			s.gesture = append(s.gesture, s.touchLocked("touchMove", cmd))
			s.mu.Unlock()
			return nil
		}
		gesture := append(s.gesture, s.touchLocked("touchUp", cmd))
		s.gesture = nil
		s.mu.Unlock()
		// The whole gesture goes out as one batch, and only its Up waits for it.
		return <-s.input.enqueue(gesture, true)
	case "key":
		// Argent sends USB HID keyboard usages, which is HID usage page 7.
		action := ActionUsageUp(7, cmd.Code)
		if pressed {
			action = ActionUsageDown(7, cmd.Code)
		}
		// Keys and buttons are acknowledged before they reach the simulator, to
		// keep Argent's own typing pace. A failure is reported on the next call.
		s.input.enqueue([]PerformAction{action}, false)
		return nil
	case "button":
		button, ok := argentButtons[cmd.Button]
		if !ok {
			return fmt.Errorf("button %s is not supported on Limrun simulators", cmd.Button)
		}
		action := ActionButtonUp(button)
		if pressed {
			action = ActionButtonDown(button)
		}
		s.input.enqueue([]PerformAction{action}, false)
		return nil
	case "rotate":
		if err := s.input.settled(); err != nil {
			return err
		}
		if err := s.client.SetOrientation(s.ctx, Orientation(cmd.Direction)); err != nil {
			return err
		}
		return s.refreshInfo(s.ctx)
	case "capture_on_demand":
		return nil
	default:
		return fmt.Errorf("parse error: unknown variant `%s`", cmd.Cmd)
	}
}

// touchLocked is touch for callers that already hold s.mu.
func (s *argentSession) touchLocked(kind string, cmd simCommand) PerformAction {
	width, height := s.info.ScreenWidth, s.info.ScreenHeight
	if s.info.DisplayWidth > 0 && s.info.DisplayHeight > 0 {
		width, height = s.info.DisplayWidth, s.info.DisplayHeight
	}
	action := PerformAction{Type: kind, X: cmd.X * width, Y: cmd.Y * height}
	if cmd.SecondX != nil && cmd.SecondY != nil {
		x2, y2 := *cmd.SecondX*width, *cmd.SecondY*height
		action.X2, action.Y2 = &x2, &y2
	}
	return action
}

// ---- HTTP -------------------------------------------------------------------

func (s *argentSession) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/ws":
		conn, err := s.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		go s.serveCommands(conn)
		return
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/media/"):
		http.ServeFile(w, r, filepath.Join(s.mediaDir, filepath.Base(r.URL.Path)))
		return
	case r.Method == http.MethodGet && r.URL.Path == "/":
		// Argent probes apiUrl to check that the provider is alive.
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	case r.Method != http.MethodPost:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ctx := r.Context()
	var out any
	var err error
	switch r.URL.Path {
	case "/api/screenshot":
		_, scaled := body["scale"]
		out, err = s.screenshot(ctx, scaled)
	case "/api/pointer":
		out = map[string]string{"status": "ok"}
	case "/api/clipboard/text":
		out, err = s.clipboard(ctx, rawString(body["text"]))
	case "/xcrun":
		var args []string
		_ = json.Unmarshal(body["args"], &args)
		out, err = s.simctl(ctx, args)
	case "/ax":
		out = s.accessibilityReply(ctx, rawString(body["command"]), body["params"])
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		// One body serves both callers: Argent reads `error`, the xcrun shim reads the rest.
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error(), "code": 1, "stdout": "", "stderr": "limrun: " + err.Error() + "\n"})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *argentSession) serveCommands(conn *websocket.Conn) {
	defer conn.Close()
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// Commands answer in order, as simulator-server does: a tap's Up waits
		// for the gesture, and a key sent after it waits behind it.
		if err = conn.WriteJSON(s.handleCommand(raw)); err != nil {
			return
		}
	}
}

func (s *argentSession) screenshot(ctx context.Context, scaled bool) (map[string]string, error) {
	if err := s.input.settled(); err != nil {
		return nil, err
	}
	// Argent omits `scale` only when it wants native pixels. Every smaller
	// scale gets the point-sized image, the nearest size the simulator serves.
	size := "full"
	if scaled {
		size = "scaled"
	}
	data, err := s.client.Screenshot(ctx, ScreenshotOptions{Format: "png", Size: size})
	if err != nil {
		return nil, err
	}
	png, err := base64.StdEncoding.DecodeString(data.Base64)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	name := fmt.Sprintf("screenshot-%d-%d.png", time.Now().UnixMilli(), len(s.mediaFiles))
	file := filepath.Join(s.mediaDir, name)
	s.mediaFiles = append(s.mediaFiles, file)
	var stale []string
	if len(s.mediaFiles) > mediaFilesKept {
		stale = s.mediaFiles[:len(s.mediaFiles)-mediaFilesKept]
		s.mediaFiles = s.mediaFiles[len(s.mediaFiles)-mediaFilesKept:]
	}
	s.mu.Unlock()
	for _, f := range stale {
		_ = os.Remove(f)
	}
	if err = os.WriteFile(file, png, 0o644); err != nil {
		return nil, err
	}
	return map[string]string{"url": s.baseURL + "/media/" + name, "path": file}, nil
}

func (s *argentSession) clipboard(ctx context.Context, text string) (map[string]string, error) {
	if err := s.input.settled(); err != nil {
		return nil, err
	}
	result, err := s.runSimctl(ctx, []string{"pbcopy", s.udid()}, []byte(text))
	if err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return map[string]string{"error": result.Stderr}, nil
	}
	return map[string]string{"status": "ok"}, nil
}

func (s *argentSession) udid() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info.UDID
}

// ---- simctl -----------------------------------------------------------------

func (s *argentSession) runSimctl(ctx context.Context, args []string, stdin []byte) (*ShimSimctlResult, error) {
	var stdout, stderr bytes.Buffer
	cmd := s.client.Simctl(ctx, args...)
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil && cmd.ExitCode() == 0 {
		return nil, err
	}
	return &ShimSimctlResult{Code: cmd.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

// simctl runs a simctl call for this simulator, as the xcrun shim forwards
// it (without `xcrun simctl`).
func (s *argentSession) simctl(ctx context.Context, args []string) (*ShimSimctlResult, error) {
	udid := s.udid()
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	var rest []string
	for i, arg := range args {
		if arg == udid {
			rest = args[i+1:]
			break
		}
	}

	if command == "spawn" {
		// Argent reads this flag to decide whether accessibility is degraded.
		// It describes Argent's own daemon, which does not run here.
		if strings.Join(rest, " ") == "defaults read com.apple.Accessibility IgnoreAXServerEntitlements" {
			return &ShimSimctlResult{Stdout: "1\n"}, nil
		}
		if len(rest) >= 2 && rest[0] == "launchctl" && rest[1] == "list" {
			// Argent parses running apps out of `launchctl list`, which the
			// simulator does not let callers run.
			apps, err := s.client.ListApps(ctx)
			if err != nil {
				return nil, err
			}
			lines := []string{"PID\tStatus\tLabel"}
			for _, app := range apps {
				if app.PID != nil {
					lines = append(lines, fmt.Sprintf("%d\t0\tUIKitApplication:%s[0000][rb-legacy]", *app.PID, app.BundleID))
				}
			}
			return &ShimSimctlResult{Stdout: strings.Join(lines, "\n") + "\n"}, nil
		}
	}
	if err := s.input.settled(); err != nil {
		return nil, err
	}
	switch command {
	case "bootstatus":
		return &ShimSimctlResult{}, nil
	case "install":
		if len(rest) == 0 {
			return &ShimSimctlResult{Code: 64, Stderr: "simctl install requires a local .app path.\n"}, nil
		}
		install, watch := true, false
		if _, err := s.client.SyncApp(ctx, rest[0], SyncAppOptions{Install: &install, Watch: &watch, LaunchMode: syncfolder.LaunchNone}); err != nil {
			return nil, err
		}
		return &ShimSimctlResult{}, nil
	}
	return s.runSimctl(ctx, args, nil)
}

// ---- accessibility ------------------------------------------------------------

// accessibilityReply answers one accessibility daemon request as {result} or {error}.
func (s *argentSession) accessibilityReply(ctx context.Context, command string, params json.RawMessage) map[string]any {
	if err := s.input.settled(); err != nil {
		return map[string]any{"error": err.Error()}
	}
	var p map[string]any
	_ = json.Unmarshal(params, &p)
	result, err := s.accessibility(ctx, command, p)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"result": result}
}

func (s *argentSession) accessibility(ctx context.Context, command string, params map[string]any) (any, error) {
	switch command {
	case "ping":
		return map[string]string{"status": "ok"}, nil
	case "live_panel":
		return map[string]any{"displayId": nil}, nil
	case "orientation":
		if err := s.refreshInfo(ctx); err != nil {
			return nil, err
		}
		width, height := s.screen()
		return map[string]any{"interfaceOrientation": s.orientation(), "screenFrame": map[string]float64{"width": width, "height": height}}, nil
	case "fill":
		if check, _ := params["check"].(bool); !check {
			// Without a selector the simulator resolves the focused field itself.
			if _, err := s.client.SetElementValue(ctx, fmt.Sprint(params["text"])); err != nil {
				return nil, errors.New("no_focused_field")
			}
			return map[string]bool{"verified": true}, nil
		}
	}

	tree, err := s.client.ElementTreeNodes(ctx, nil, ElementTreeOptions{VisibleOnly: true})
	if err != nil {
		return nil, err
	}
	width, height := s.screen()
	screen := ArgentScreen{Width: width, Height: height}
	alert := someNode(tree, func(n ElementTreeNode) bool { return n.Type == "Alert" || n.Type == "Sheet" })
	base := map[string]any{
		"alertVisible":         alert,
		"screenFrame":          map[string]float64{"width": width, "height": height},
		"interfaceOrientation": s.orientation(),
	}
	switch command {
	case "alert_check":
		return map[string]bool{"alertVisible": alert}, nil
	case "fill":
		text := fmt.Sprint(params["text"])
		return map[string]bool{"verified": someNode(tree, func(n ElementTreeNode) bool { return n.AXValue != nil && *n.AXValue == text })}, nil
	case "foreground_app":
		app, err := s.foregroundApp(ctx, tree)
		if err != nil {
			return nil, err
		}
		app["alertVisible"] = alert
		return app, nil
	case "describe":
		base["elements"] = ArgentElements(tree, screen)
		return base, nil
	case "tree":
		app, err := s.foregroundApp(ctx, tree)
		if err != nil {
			return nil, err
		}
		bundleID, _ := app["bundleId"].(string)
		base["nodes"] = argentTreeNodes(tree, screen, bundleID)
		base["truncated"] = false
		base["treeVersion"] = 1
		if bundleID != "" {
			base["foregroundApp"] = bundleID
		}
		return base, nil
	}
	// Argent treats this code as "the daemon predates the command".
	return nil, errors.New("unknown_command")
}

func (s *argentSession) foregroundApp(ctx context.Context, tree []ElementTreeNode) (map[string]any, error) {
	out := map[string]any{}
	if len(tree) == 0 {
		return out, nil
	}
	root := tree[0]
	apps, err := s.client.ListApps(ctx)
	if err != nil {
		return nil, err
	}
	name := ""
	for _, app := range apps {
		if app.PID != nil && *app.PID == root.PID {
			out["bundleId"] = app.BundleID
			name = app.Name
			break
		}
	}
	if root.AXLabel != nil {
		name = *root.AXLabel
	}
	if name != "" {
		out["name"] = name
	}
	return out, nil
}

// ---- tree translation ----------------------------------------------------------

// ArgentScreen is the screen size in points that Argent frames are fractions of.
type ArgentScreen struct{ Width, Height float64 }

// ArgentElement is one element of Argent's `describe` list.
type ArgentElement struct {
	Label      string        `json:"label,omitempty"`
	Hint       string        `json:"hint,omitempty"`
	Frame      ElementBounds `json:"frame"`
	Traits     []string      `json:"traits"`
	Value      string        `json:"value,omitempty"`
	Identifier string        `json:"identifier,omitempty"`
}

// ElementBounds is a frame as fractions of the screen.
type ElementBounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Argent derives an element's role from these trait names.
var traitByType = map[string]string{
	"Button":          "button",
	"Cell":            "button",
	"Switch":          "button",
	"StaticText":      "staticText",
	"Heading":         "header",
	"Link":            "link",
	"Image":           "image",
	"TextField":       "textEntry",
	"SecureTextField": "textEntry",
	"TextView":        "textEntry",
	"SearchField":     "searchField",
	"TabBar":          "tabBar",
	"Slider":          "adjustable",
}

var containerTypes = map[string]bool{
	"Application": true, "Window": true, "Group": true, "Other": true,
	"ScrollView": true, "Table": true, "CollectionView": true,
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toArgentElement(node ElementTreeNode, screen ArgentScreen) (ArgentElement, bool) {
	f := node.Frame
	if f.Width <= 0 || f.Height <= 0 {
		return ArgentElement{}, false
	}
	var traits []string
	seen := map[string]bool{}
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			traits = append(traits, t)
		}
	}
	for _, t := range node.Traits {
		if t != "" {
			add(strings.ToLower(t[:1]) + t[1:])
		}
	}
	if t, ok := traitByType[node.Type]; ok {
		add(t)
	}
	if !node.Enabled {
		add("notEnabled")
	}
	if traits == nil {
		traits = []string{}
	}
	label := deref(node.AXLabel)
	if label == "" {
		label = deref(node.Title)
	}
	return ArgentElement{
		Label: label,
		Hint:  deref(node.Help),
		// Argent expects frames as fractions of the screen.
		Frame:      ElementBounds{X: f.X / screen.Width, Y: f.Y / screen.Height, Width: f.Width / screen.Width, Height: f.Height / screen.Height},
		Traits:     traits,
		Value:      deref(node.AXValue),
		Identifier: deref(node.AXUniqueID),
	}, true
}

// ArgentElements is the flat element list behind Argent's `describe`: every
// node except unnamed containers.
func ArgentElements(tree []ElementTreeNode, screen ArgentScreen) []ArgentElement {
	elements := []ArgentElement{}
	var visit func([]ElementTreeNode)
	visit = func(nodes []ElementTreeNode) {
		for _, node := range nodes {
			named := deref(node.AXLabel) != "" || deref(node.AXValue) != "" || deref(node.Title) != "" || deref(node.AXUniqueID) != ""
			if !containerTypes[node.Type] || named {
				if el, ok := toArgentElement(node, screen); ok {
					elements = append(elements, el)
				}
			}
			visit(node.Children)
		}
	}
	visit(tree)
	return elements
}

// argentTreeNodes is the nested tree behind Argent's `ui-tree`, in document
// order with parent links.
func argentTreeNodes(tree []ElementTreeNode, screen ArgentScreen, rootBundleID string) []map[string]any {
	nodes := []map[string]any{}
	var visit func([]ElementTreeNode, int)
	visit = func(children []ElementTreeNode, parent int) {
		for _, node := range children {
			index := len(nodes)
			entry := map[string]any{"index": index, "roleDescription": node.RoleDescription}
			if parent < 0 {
				if rootBundleID != "" {
					entry["bundleId"] = rootBundleID
				}
			} else {
				entry["parentIndex"] = parent
			}
			if el, ok := toArgentElement(node, screen); ok {
				raw, _ := json.Marshal(el)
				_ = json.Unmarshal(raw, &entry)
			}
			nodes = append(nodes, entry)
			visit(node.Children, index)
		}
	}
	visit(tree, -1)
	return nodes
}

func someNode(tree []ElementTreeNode, matches func(ElementTreeNode) bool) bool {
	for _, node := range tree {
		if matches(node) || someNode(node.Children, matches) {
			return true
		}
	}
	return false
}

// simctlDeviceFor is this simulator as an entry of `xcrun simctl list devices --json`.
func simctlDeviceFor(info *DeviceInfo) map[string]any {
	version := info.OSVersion
	if version == "" {
		version = "26.0"
	}
	return map[string]any{
		"runtime": "com.apple.CoreSimulator.SimRuntime.iOS-" + strings.ReplaceAll(version, ".", "-"),
		"entry": map[string]any{
			"udid":                 info.UDID,
			"name":                 info.Model,
			"state":                "Booted",
			"isAvailable":          true,
			"deviceTypeIdentifier": "com.apple.CoreSimulator.SimDeviceType." + nonAlphanumeric.ReplaceAllString(info.Model, "-"),
		},
	}
}

var nonAlphanumeric = regexp.MustCompile(`[^A-Za-z0-9]+`)

func rawString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	body, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
