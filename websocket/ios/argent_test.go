package ios

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const argentTestUDID = "A1B2C3D4-0000-4000-8000-000000000001"

// A 1x1 PNG, so the screenshot the bridge writes is a real image file.
const argentTestPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func strPtr(s string) *string { return &s }

var argentSettingsTree = []ElementTreeNode{{
	Type: "Application", AXLabel: strPtr("Settings"), Enabled: true, PID: 77,
	Frame: ElementTreeFrame{Width: 400, Height: 800},
	Children: []ElementTreeNode{{
		Type: "Other", Enabled: true, Frame: ElementTreeFrame{Width: 400, Height: 800},
		Children: []ElementTreeNode{
			{Type: "Button", AXLabel: strPtr("General"), AXUniqueID: strPtr("general-cell"), Enabled: true,
				Frame: ElementTreeFrame{X: 100, Y: 200, Width: 200, Height: 80}},
			{Type: "TextField", AXLabel: strPtr("Search"), AXValue: strPtr("wifi"), Enabled: false,
				Frame: ElementTreeFrame{Y: 400, Width: 400, Height: 40}},
		},
	}},
}}

// argentFake is an instance that answers the requests the bridge sends.
type argentFake struct {
	mu        sync.Mutex
	calls     []string
	performed [][]map[string]any
	simctl    [][]string
}

func (f *argentFake) record(call string) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}

func (f *argentFake) handle(ws *websocket.Conn, req map[string]json.RawMessage) {
	id := field(req, "id")
	switch field(req, "type") {
	case "deviceInfo":
		_ = ws.WriteJSON(map[string]any{"id": id, "type": "deviceInfoResult", "udid": argentTestUDID,
			"screenWidth": 400, "screenHeight": 800, "model": "iPhone 16 Pro", "osVersion": "26.4"})
	case "performActions":
		var actions []map[string]any
		_ = json.Unmarshal(req["actions"], &actions)
		// Slower than the test's next request, so ordering is what gets asserted.
		time.Sleep(40 * time.Millisecond)
		f.mu.Lock()
		f.calls = append(f.calls, "performActions")
		f.performed = append(f.performed, actions)
		f.mu.Unlock()
		_ = ws.WriteJSON(map[string]any{"id": id, "results": []any{}})
	case "screenshot":
		f.record("screenshot")
		_ = ws.WriteJSON(map[string]any{"id": id, "base64": argentTestPNG, "width": 1, "height": 1})
	case "elementTree":
		tree, _ := json.Marshal(argentSettingsTree)
		_ = ws.WriteJSON(map[string]any{"id": id, "json": string(tree)})
	case "listApps":
		apps, _ := json.Marshal([]map[string]any{{"bundleId": "com.apple.Preferences", "name": "Settings", "installType": "System", "pid": 77}})
		_ = ws.WriteJSON(map[string]any{"id": id, "apps": string(apps)})
	case "simctl":
		var args []string
		_ = json.Unmarshal(req["args"], &args)
		f.mu.Lock()
		f.simctl = append(f.simctl, args)
		f.mu.Unlock()
		// "simctl-ok\n" in base64.
		_ = ws.WriteJSON(map[string]any{"type": "simctlStream", "id": id, "stdout": "c2ltY3RsLW9rCg==", "exitCode": 0})
	default:
		_ = ws.WriteJSON(map[string]any{"id": id})
	}
}

func startTestBridge(t *testing.T) (*ArgentBridge, *argentFake, string) {
	t.Helper()
	fake := &argentFake{}
	client := fixtureClient(t, fake.handle)
	providersDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bridge, err := StartArgentBridge(ctx, client, ArgentBridgeOptions{ProvidersDir: providersDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bridge.Close() })
	var descriptor struct {
		Devices []struct {
			SimulatorServer struct {
				APIURL string `json:"apiUrl"`
			} `json:"simulatorServer"`
		} `json:"devices"`
	}
	data, _ := os.ReadFile(bridge.DescriptorPath)
	_ = json.Unmarshal(data, &descriptor)
	return bridge, fake, descriptor.Devices[0].SimulatorServer.APIURL
}

func dialCommands(t *testing.T, apiURL string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(apiURL, "http:", "ws:", 1)+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func sendCommand(t *testing.T, conn *websocket.Conn, cmd map[string]any) map[string]any {
	t.Helper()
	if err := conn.WriteJSON(cmd); err != nil {
		t.Fatal(err)
	}
	var reply map[string]any
	if err := conn.ReadJSON(&reply); err != nil {
		t.Fatal(err)
	}
	return reply
}

func shimPath(bridge *ArgentBridge) string {
	return filepath.Join(strings.SplitN(bridge.Env["PATH"], string(os.PathListSeparator), 2)[0], "xcrun")
}

func requireNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
}

func TestArgentBridgePublishesAndWithdrawsDescriptor(t *testing.T) {
	bridge, _, apiURL := startTestBridge(t)
	var descriptor map[string]any
	data, _ := os.ReadFile(bridge.DescriptorPath)
	if err := json.Unmarshal(data, &descriptor); err != nil {
		t.Fatal(err)
	}
	if descriptor["id"] != "limrun-a1b2c3d4" || descriptor["schemaVersion"] != float64(1) {
		t.Fatalf("descriptor=%v", descriptor)
	}
	device := descriptor["devices"].([]any)[0].(map[string]any)
	if device["nativeId"] != argentTestUDID || device["platform"] != "ios" || len(device["capabilities"].([]any)) != 3 {
		t.Fatalf("device=%v", device)
	}
	if bridge.DeviceID != "ext:limrun-a1b2c3d4:"+argentTestUDID {
		t.Fatalf("DeviceID=%s", bridge.DeviceID)
	}
	// Argent drops a device whose apiUrl does not answer.
	resp, err := http.Get(apiURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("liveness probe: %v %v", resp, err)
	}
	resp.Body.Close()

	bridge.Close()
	if _, err := os.Stat(bridge.DescriptorPath); !os.IsNotExist(err) {
		t.Fatalf("descriptor still present: %v", err)
	}
}

func TestArgentBridgeReplaysTapAsOneBatchInScreenPoints(t *testing.T) {
	_, fake, apiURL := startTestBridge(t)
	conn := dialCommands(t, apiURL)
	if reply := sendCommand(t, conn, map[string]any{"id": "1", "cmd": "touch", "type": "Down", "x": 0.5, "y": 0.25}); reply["id"] != "1" || reply["status"] != "ok" {
		t.Fatalf("down reply=%v", reply)
	}
	if reply := sendCommand(t, conn, map[string]any{"id": "2", "cmd": "touch", "type": "Up", "x": 0.5, "y": 0.25}); reply["id"] != "2" || reply["status"] != "ok" {
		t.Fatalf("up reply=%v", reply)
	}
	// The Up was acknowledged only after the gesture ran.
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.performed) != 1 {
		t.Fatalf("batches=%v", fake.performed)
	}
	var touches []map[string]any
	for _, action := range fake.performed[0] {
		if action["type"] != "wait" {
			touches = append(touches, action)
		}
	}
	if len(touches) != 2 || touches[0]["type"] != "touchDown" || touches[0]["x"] != float64(200) || touches[0]["y"] != float64(200) || touches[1]["type"] != "touchUp" {
		t.Fatalf("touches=%v", touches)
	}
}

func TestArgentBridgeRejectsUnknownCommandWithoutID(t *testing.T) {
	_, _, apiURL := startTestBridge(t)
	reply := sendCommand(t, dialCommands(t, apiURL), map[string]any{"id": "9", "cmd": "teleport"})
	if _, hasID := reply["id"]; hasID || reply["status"] != "error" || reply["message"] != "parse error: unknown variant `teleport`" {
		t.Fatalf("reply=%v", reply)
	}
}

func TestArgentBridgeScreenshotWaitsForEarlyAcknowledgedKeys(t *testing.T) {
	_, fake, apiURL := startTestBridge(t)
	conn := dialCommands(t, apiURL)
	sendCommand(t, conn, map[string]any{"id": "1", "cmd": "key", "code": 4, "direction": "Down"})
	sendCommand(t, conn, map[string]any{"id": "2", "cmd": "key", "code": 4, "direction": "Up"})
	resp, err := http.Post(apiURL+"/api/screenshot", "application/json", strings.NewReader(`{"scale":0.25}`))
	if err != nil {
		t.Fatal(err)
	}
	var shot map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&shot)
	resp.Body.Close()

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.calls[len(fake.calls)-1] != "screenshot" {
		t.Fatalf("calls=%v", fake.calls)
	}
	var keys []map[string]any
	for _, batch := range fake.performed {
		keys = append(keys, batch...)
	}
	if len(keys) != 2 || keys[0]["type"] != "usageDown" || keys[0]["usagePage"] != float64(7) || keys[0]["usage"] != float64(4) || keys[1]["type"] != "usageUp" {
		t.Fatalf("keys=%v", keys)
	}
	data, err := os.ReadFile(shot["path"])
	if err != nil || string(data[1:4]) != "PNG" {
		t.Fatalf("screenshot file: %v", err)
	}
}

func TestArgentShimListsAndForwardsSimctl(t *testing.T) {
	requireNode(t)
	bridge, fake, _ := startTestBridge(t)
	run := func(args ...string) (string, int) {
		cmd := exec.Command("node", append([]string{shimPath(bridge)}, args...)...)
		// No real xcrun, as on a Linux agent machine.
		cmd.Env = append(os.Environ(), "LIMRUN_REAL_XCRUN=/nonexistent/xcrun")
		out, err := cmd.Output()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		}
		return string(out), code
	}

	listing, _ := run("simctl", "list", "devices", "--json")
	var parsed struct {
		Devices map[string][]map[string]any `json:"devices"`
	}
	if err := json.Unmarshal([]byte(listing), &parsed); err != nil {
		t.Fatalf("listing=%q: %v", listing, err)
	}
	entries := parsed.Devices["com.apple.CoreSimulator.SimRuntime.iOS-26-4"]
	if len(entries) != 1 || entries[0]["udid"] != argentTestUDID || entries[0]["state"] != "Booted" {
		t.Fatalf("entries=%v", entries)
	}

	// Argent prefixes a device set when one is configured.
	out, code := run("simctl", "--set", "/sets/a", "launch", argentTestUDID, "com.example.app")
	if code != 0 || out != "simctl-ok\n" {
		t.Fatalf("launch out=%q code=%d", out, code)
	}
	fake.mu.Lock()
	forwarded := fake.simctl
	fake.mu.Unlock()
	if len(forwarded) != 1 || strings.Join(forwarded[0], " ") != "launch "+argentTestUDID+" com.example.app" {
		t.Fatalf("forwarded=%v", forwarded)
	}

	if _, code = run("simctl", "launch", "FFFFFFFF-0000-4000-8000-000000000000", "x"); code != 127 {
		t.Fatalf("other simulator code=%d", code)
	}
}

func TestArgentShimAnswersAsAccessibilityDaemon(t *testing.T) {
	requireNode(t)
	bridge, _, _ := startTestBridge(t)
	// Argent's TCP endpoint; a unix socket path under the test temp dir can exceed sun_path.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	daemon := exec.Command("node", shimPath(bridge), "simctl", "spawn", argentTestUDID, "/argent/bin/darwin/tcp/ax-service", "--port", port, "--timeout", "3600")
	if err = daemon.Start(); err != nil {
		t.Fatal(err)
	}
	defer daemon.Process.Kill()

	_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte(`{"id":1,"command":"describe"}` + "\n" + `{"id":2,"command":"teleport"}` + "\n"))
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	replies := map[float64]map[string]any{}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for len(replies) < 2 && scanner.Scan() {
		var reply map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &reply); err != nil {
			t.Fatal(err)
		}
		replies[reply["id"].(float64)] = reply
	}

	describe := replies[1]["result"].(map[string]any)
	if frame := describe["screenFrame"].(map[string]any); frame["width"] != float64(400) || frame["height"] != float64(800) {
		t.Fatalf("screenFrame=%v", frame)
	}
	var labels []string
	for _, element := range describe["elements"].([]any) {
		labels = append(labels, element.(map[string]any)["label"].(string))
	}
	if strings.Join(labels, ",") != "Settings,General,Search" {
		t.Fatalf("labels=%v", labels)
	}
	if replies[2]["error"] != "unknown_command" {
		t.Fatalf("unknown command reply=%v", replies[2])
	}
}

func TestArgentElementsMapsFramesAndTraits(t *testing.T) {
	elements := ArgentElements(argentSettingsTree, ArgentScreen{Width: 400, Height: 800})
	// The unnamed "Other" container is dropped; the labelled application stays.
	if len(elements) != 3 {
		t.Fatalf("elements=%v", elements)
	}
	general := elements[1]
	if general.Label != "General" || general.Identifier != "general-cell" || general.Frame != (ElementBounds{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.1}) || strings.Join(general.Traits, ",") != "button" {
		t.Fatalf("general=%+v", general)
	}
	if search := elements[2]; search.Value != "wifi" || strings.Join(search.Traits, ",") != "textEntry,notEnabled" {
		t.Fatalf("search=%+v", search)
	}
}
