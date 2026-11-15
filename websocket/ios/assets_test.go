package ios

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestInstallAppSources(t *testing.T) {
	frames := make(chan map[string]json.RawMessage, 3)
	client := fixtureClient(t, func(ws *websocket.Conn, req map[string]json.RawMessage) {
		frames <- req
		reply := map[string]string{"type": "appInstallationResult", "id": field(req, "id"), "bundleId": "com.example.app"}
		if name := field(req, "assetName"); name != "" {
			reply["assetName"] = name
		} else {
			reply["url"] = field(req, "url")
		}
		if field(req, "assetName") == "denied.zip" {
			reply["error"] = "asset read denied"
		}
		_ = ws.WriteJSON(reply)
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	named, err := client.InstallAppByName(ctx, "team/app:prod+版本.zip", &AppInstallationOptions{MD5: "checksum", LaunchMode: LaunchModeRelaunchIfRunning})
	if err != nil {
		t.Fatal(err)
	}
	if named.AssetName != "team/app:prod+版本.zip" || named.URL != "" || named.BundleID != "com.example.app" {
		t.Fatalf("named result: %+v", named)
	}
	url, err := client.InstallApp(ctx, "https://storage.example/app.zip", nil)
	if err != nil {
		t.Fatal(err)
	}
	if url.URL != "https://storage.example/app.zip" || url.AssetName != "" {
		t.Fatalf("URL result: %+v", url)
	}
	if _, err := client.InstallAppByName(ctx, "denied.zip", nil); err == nil || !strings.Contains(err.Error(), "read denied") {
		t.Fatalf("denial: %v", err)
	}
	nameFrame, urlFrame := <-frames, <-frames
	if _, ok := nameFrame["url"]; ok {
		t.Fatal("named install sent url")
	}
	if _, ok := urlFrame["assetName"]; ok {
		t.Fatal("URL install sent assetName")
	}
	if field(nameFrame, "id") == "" || field(nameFrame, "id") == field(urlFrame, "id") {
		t.Fatal("missing or repeated ID")
	}
	if field(nameFrame, "md5") != "checksum" || field(nameFrame, "launchMode") != "RelaunchIfRunning" {
		t.Fatalf("options: %v", nameFrame)
	}
	if _, err := new(Client).InstallAppByName(ctx, "", nil); err == nil {
		t.Fatal("accepted empty asset name")
	}
}
