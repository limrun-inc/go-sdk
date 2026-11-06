package ios

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestXcrunShimExecutesDeviceEnumeration(t *testing.T) {
	if _, e := exec.LookPath("node"); e != nil {
		t.Skip("node unavailable")
	}
	server, e := StartXcrunShimServer(nil, "test-udid")
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	dir, e := CreateXcrunShim(XcrunShimOptions{ShimURL: server.URL, UDID: "test-udid"})
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	data, e := exec.Command(filepath.Join(dir, "xcrun"), "simctl", "list", "-j").Output()
	if e != nil {
		t.Fatal(e)
	}
	if !json.Valid(data) || !strings.Contains(string(data), "test-udid") {
		t.Fatal(string(data))
	}
	output := filepath.Join(t.TempDir(), "devices.json")
	data, e = exec.Command(filepath.Join(dir, "xcrun"), "devicectl", "--json-output", output, "list", "devices").CombinedOutput()
	if e != nil {
		t.Fatalf("%v %s", e, data)
	}
	data, e = os.ReadFile(output)
	if e != nil || string(data) != `{"result":{"devices":[]}}` {
		t.Fatalf("%s %v", data, e)
	}
	result, e := handleShimmedXcrun(context.Background(), nil, "test-udid", []string{"simctl", "get_app_container", "booted", "example"})
	if e != nil || result.Code != 64 {
		t.Fatalf("%+v %v", result, e)
	}
}
