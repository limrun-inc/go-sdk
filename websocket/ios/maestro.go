package ios

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/limrun-inc/go-sdk/internal/proxy"
)

const MaestroRunnerBundleID = "dev.mobile.maestro-driver-iosUITests.xctrunner"
const MaestroRunnerPort = 22087

type MaestroRun struct {
	MaestroVersion string
	DriverPort     int
	Env            map[string]string
	Args           []string
}

func PrepareMaestroRun(ctx context.Context, client *Client, runnerURL string) (*MaestroRun, error) {
	command := exec.CommandContext(ctx, "maestro", "--version")
	if runtime.GOOS == "windows" {
		command = exec.CommandContext(ctx, "cmd", "/c", "maestro", "--version")
	}
	data, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run maestro --version; install the Maestro CLI first: https://docs.maestro.dev/getting-started/installing-maestro: %w", err)
	}
	version := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(line) {
			version = line
			break
		}
	}
	if version == "" {
		return nil, fmt.Errorf("could not parse the Maestro CLI version from maestro --version output")
	}
	shim, err := client.StartXcrunShim(ctx)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(version, ".")
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	flag := major > 2 || major == 2 && minor >= 6
	driverPort := 7001
	if flag {
		listener, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			return nil, e
		}
		driverPort = listener.Addr().(*net.TCPAddr).Port
		listener.Close()
	}
	proxyPort, err := client.StartForwardHTTPProxy(ForwardHTTPProxyOptions{RemoteBaseURL: runnerURL, MatchPort: driverPort})
	if err != nil {
		return nil, err
	}
	info, err := client.DeviceInfo(ctx)
	if err != nil {
		return nil, err
	}
	maestroOpts := fmt.Sprintf("-Dhttp.proxyHost=127.0.0.1 -Dhttp.proxyPort=%d -Dhttp.nonProxyHosts=", proxyPort)
	if current := os.Getenv("MAESTRO_OPTS"); current != "" {
		maestroOpts = current + " " + maestroOpts
	}
	args := []string{"--platform", "ios", "--device", info.UDID, "--no-reinstall-driver"}
	if flag {
		args = append(args, "--driver-host-port", strconv.Itoa(driverPort))
	}
	return &MaestroRun{MaestroVersion: version, DriverPort: driverPort, Env: map[string]string{"MAESTRO_OPTS": maestroOpts, "PATH": shim + string(os.PathListSeparator) + os.Getenv("PATH"), "USE_XCODE_TEST_RUNNER": "1"}, Args: args}, nil
}
func IsMaestroRunnerRunning(ctx context.Context, runnerURL, token string) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", runnerURL+"/status", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := proxy.Client().Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode >= 200 && res.StatusCode < 300
}
func WaitForMaestroRunner(ctx context.Context, runnerURL, token string, timeout ...time.Duration) bool {
	duration := 15 * time.Second
	if len(timeout) > 0 {
		duration = timeout[0]
	}
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	for ctx.Err() == nil {
		if IsMaestroRunnerRunning(ctx, runnerURL, token) {
			return true
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
	return false
}
