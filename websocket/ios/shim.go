package ios

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed shim/xcrun.js
var xcrunSource string

//go:embed shim/devices.json
var shimDeviceList string

type XcrunShimServer struct {
	URL    string
	server *http.Server
}

func (s *XcrunShimServer) Close() error { return s.server.Close() }

type XcrunShim struct {
	Dir    string
	server *XcrunShimServer
	once   sync.Once
}

func (s *XcrunShim) Close() error {
	var err error
	s.once.Do(func() {
		err = s.server.Close()
		if e := os.RemoveAll(s.Dir); err == nil {
			err = e
		}
	})
	return err
}

type XcrunShimOptions struct{ ShimURL, UDID string }
type ShimSimctlResult struct {
	Code   int    `json:"code"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// CreateXcrunShim writes the same Node executable used by the TypeScript SDK.
// It delegates unrelated xcrun commands to LIMRUN_REAL_XCRUN or /usr/bin/xcrun.
func CreateXcrunShim(options ...XcrunShimOptions) (string, error) {
	source := xcrunSource
	if len(options) > 0 {
		u, _ := json.Marshal(options[0].ShimURL)
		id, _ := json.Marshal(options[0].UDID)
		source = strings.Replace(source, "const shimUrl = process.env.LIMRUN_XCRUN_SHIM_URL;", "const shimUrl = "+string(u)+";", 1)
		source = strings.Replace(source, "const udid = process.env.LIMRUN_IOS_UDID;", "const udid = "+string(id)+";", 1)
	}
	dir, err := os.MkdirTemp("", "limrun-xcrun-shim-")
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(dir, "xcrun"), []byte(source), 0755); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}
func StartXcrunShim(ctx context.Context, client *Client) (*XcrunShim, error) {
	info, err := client.DeviceInfo(ctx)
	if err != nil {
		return nil, err
	}
	server, err := StartXcrunShimServer(client, info.UDID)
	if err != nil {
		return nil, err
	}
	dir, err := CreateXcrunShim(XcrunShimOptions{ShimURL: server.URL, UDID: info.UDID})
	if err != nil {
		server.Close()
		return nil, err
	}
	return &XcrunShim{Dir: dir, server: server}, nil
}
func (c *Client) StartXcrunShim(ctx context.Context) (string, error) {
	shim, err := StartXcrunShim(ctx, c)
	if err != nil {
		return "", err
	}
	c.cleanupsMu.Lock()
	if c.closed.Load() {
		c.cleanupsMu.Unlock()
		shim.Close()
		return "", ErrNotConnected
	}
	c.cleanups = append(c.cleanups, func() { shim.Close() })
	c.cleanupsMu.Unlock()
	return shim.Dir, nil
}
func StartXcrunShimServer(client *Client, udid string) (*XcrunShimServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	server := &XcrunShimServer{URL: "http://" + listener.Addr().String() + "/xcrun"}
	server.server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := ShimSimctlResult{Code: 127, Stderr: "not found"}
		status := 404
		if r.Method == "POST" && r.URL.RequestURI() == "/xcrun" {
			status = 200
			var body struct {
				Args []string `json:"args"`
			}
			err := json.NewDecoder(r.Body).Decode(&body)
			if err == nil || errors.Is(err, io.EOF) {
				result, err = handleShimmedXcrun(r.Context(), client, udid, body.Args)
			}
			if err != nil {
				result = ShimSimctlResult{Code: 1, Stderr: "limrun xcrun shim failed to execute the requested command."}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(result)
	})}
	go server.server.Serve(listener)
	return server, nil
}
func handleShimmedXcrun(ctx context.Context, client *Client, udid string, args []string) (ShimSimctlResult, error) {
	result := func(code int, message string) (ShimSimctlResult, error) {
		return ShimSimctlResult{Code: code, Stderr: message}, nil
	}
	if len(args) > 0 && args[0] == "devicectl" {
		var positionals []string
		jsonPath := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "--json-output" {
				i++
				if i < len(args) {
					jsonPath = args[i]
				}
				continue
			}
			if strings.HasPrefix(args[i], "-") {
				continue
			}
			positionals = append(positionals, args[i])
		}
		if len(positionals) < 2 || positionals[0] != "list" || positionals[1] != "devices" {
			return result(127, "unsupported devicectl command: "+strings.Join(args, " "))
		}
		payload := `{"result":{"devices":[]}}`
		if jsonPath != "" && jsonPath != "-" {
			if err := os.WriteFile(jsonPath, []byte(payload), 0644); err != nil {
				return result(1, fmt.Sprintf("could not write devicectl output to %s: %v", jsonPath, err))
			}
			return ShimSimctlResult{}, nil
		}
		return ShimSimctlResult{Stdout: payload + "\n"}, nil
	}
	if len(args) == 0 || args[0] != "simctl" {
		return result(127, "unsupported xcrun command: "+strings.Join(args, " "))
	}
	simctlArgs := args[1:]
	command := ""
	target := ""
	if len(simctlArgs) > 0 {
		command = simctlArgs[0]
	}
	if len(simctlArgs) > 1 {
		target = simctlArgs[1]
	}
	if command == "launch" {
		target = ""
		for _, arg := range simctlArgs[1:] {
			if !strings.HasPrefix(arg, "-") {
				target = arg
				break
			}
		}
	}
	if command == "list" {
		jsonOutput := false
		for _, arg := range simctlArgs {
			jsonOutput = jsonOutput || arg == "-j"
		}
		if !jsonOutput {
			return result(64, "limrun xcrun shim only supports `xcrun simctl list -j`.")
		}
		name := os.Getenv("LIMRUN_IOS_DEVICE_NAME")
		if name == "" {
			name = "Limrun iPhone"
		}
		runtime := os.Getenv("LIMRUN_IOS_RUNTIME_NAME")
		if runtime == "" {
			runtime = "iOS 18.0"
		}
		escape := func(s string) string { b, _ := json.Marshal(s); return string(b[1 : len(b)-1]) }
		list := strings.NewReplacer("__UDID__", escape(udid), "__DEVICE_NAME__", escape(name), "__RUNTIME_NAME__", escape(runtime)).Replace(shimDeviceList)
		return ShimSimctlResult{Stdout: list + "\n"}, nil
	}
	if target != udid && target != "booted" {
		return result(64, fmt.Sprintf("unsupported non-Limrun simctl target '%s'.", target))
	}
	switch command {
	case "listapps":
		apps, err := client.ListApps(ctx)
		if err != nil {
			return ShimSimctlResult{}, err
		}
		entries := map[string]map[string]string{}
		for _, app := range apps {
			entry := map[string]string{"CFBundleIdentifier": app.BundleID}
			if app.Name != "" {
				entry["CFBundleName"] = app.Name
			}
			entries[app.BundleID] = entry
		}
		data, _ := json.Marshal(entries)
		return ShimSimctlResult{Stdout: string(data) + "\n"}, nil
	case "install":
		if len(simctlArgs) < 3 || simctlArgs[2] == "" {
			return result(64, "simctl install requires a local .app path.")
		}
		install, watch := true, false
		_, err := client.SyncApp(ctx, simctlArgs[2], SyncAppOptions{Install: &install, Watch: &watch})
		return ShimSimctlResult{}, err
	case "get_app_container":
		return result(64, "limrun xcrun shim does not support get_app_container because upstream tools expect a local filesystem path.")
	case "keychain":
		return result(64, "limrun xcrun shim does not support simctl keychain in v1.")
	case "io":
		return result(64, "limrun xcrun shim does not support simctl io recordVideo in v1.")
	case "push", "addmedia":
		return result(64, "limrun xcrun shim does not support path-bearing simctl "+command+" in v1.")
	case "openurl", "launch", "terminate", "uninstall", "privacy", "location", "status_bar":
		var stdout, stderr bytes.Buffer
		cmd := client.Simctl(ctx, simctlArgs...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil && cmd.ExitCode() == 0 {
			return ShimSimctlResult{}, err
		}
		return ShimSimctlResult{Code: cmd.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}, nil
	default:
		return result(64, "limrun xcrun shim does not support simctl "+command+".")
	}
}
