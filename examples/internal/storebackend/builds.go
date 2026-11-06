package storebackend

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type Request struct {
	ProjectPath       string `json:"projectPath"`
	TeamID            string `json:"teamId"`
	BundleID          string `json:"bundleId"`
	DeviceUDID        string `json:"deviceUDID"`
	SigningMode       string `json:"signingMode"`
	Scheme            string `json:"scheme"`
	SecretsDir        string `json:"secretsDir"`
	WebhookURL        string `json:"webhookUrl"`
	PackageName       string `json:"packageName"`
	GoogleAccessToken string `json:"googleAccessToken"`
	Track             string `json:"track"`
}

func (r Request) Validate(kind string) error {
	if r.ProjectPath == "" || r.WebhookURL == "" {
		return fmt.Errorf("projectPath and webhookUrl are required")
	}
	if kind == "android" {
		if r.PackageName == "" || r.GoogleAccessToken == "" {
			return fmt.Errorf("packageName and googleAccessToken are required")
		}
		return nil
	}
	if r.TeamID == "" || r.BundleID == "" {
		return fmt.Errorf("teamId and bundleId are required")
	}
	if kind == "install" && r.DeviceUDID == "" {
		return fmt.Errorf("deviceUDID is required")
	}
	if kind == "publish" && r.SigningMode != "cloud" && r.SigningMode != "manual" {
		return fmt.Errorf("signingMode must be cloud or manual")
	}
	return nil
}
func materialize(dir, name, encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode %s: %w", name, err)
	}
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, data, 0600)
}

// Start mirrors the TypeScript examples' detached CLI workflows. The SDK calls
// for scoped tokens and asset lookup are in each backend's main.go.
func (j *Jobs) Start(ctx context.Context, store *SecretStore, kind string, request Request) (string, error) {
	if err := request.Validate(kind); err != nil {
		return "", err
	}
	var key, certificate, profile *Secret
	var err error
	if kind == "android" {
		key, err = store.Get("androidSigningKey", request.PackageName+"/UPLOAD", request.SecretsDir)
		if err != nil {
			return "", err
		}
		if key == nil {
			return "", fmt.Errorf("no upload keystore for %s; run Connect first", request.PackageName)
		}
		for _, field := range []string{"keystoreBase64", "keystorePassword", "keyAlias", "keyPassword"} {
			if key.Data[field] == "" {
				return "", fmt.Errorf("upload keystore is missing %s", field)
			}
		}
	} else {
		if kind == "publish" {
			key, err = store.Get("appStoreConnectApiKey", request.TeamID+"/APP_STORE_CONNECT_API_KEY", request.SecretsDir)
			if err != nil {
				return "", err
			}
			if key == nil {
				return "", fmt.Errorf("no App Store Connect API key; run Connect first")
			}
			if request.SigningMode == "cloud" && key.Data["issuerId"] == "" {
				return "", fmt.Errorf("cloud signing requires a team API key with an issuer ID")
			}
		}
		if kind == "install" || request.SigningMode == "manual" {
			certificate, profile, err = store.SigningCredentials(request, kind == "install")
			if err != nil {
				return "", err
			}
		}
	}
	workDir, err := os.MkdirTemp("", "limrun-"+kind+"-")
	if err != nil {
		return "", err
	}
	spawned := false
	defer func() {
		if !spawned {
			os.RemoveAll(workDir)
		}
	}()
	var args []string
	env := map[string]string{}
	var updates []string
	if kind == "android" {
		path, err := materialize(workDir, "upload.p12", key.Data["keystoreBase64"])
		if err != nil {
			return "", err
		}
		track := request.Track
		if track == "" {
			track = "internal"
		}
		args = []string{
			"gradle",
			"build",
			request.ProjectPath,
			"--keystore",
			path,
			"--key-alias",
			key.Data["keyAlias"],
			"--upload-to-playstore",
			"--playstore-package",
			request.PackageName,
			"--playstore-track",
			track,
			"--auto-version-code",
		}
		env = map[string]string{
			"LIM_KEYSTORE_PASSWORD":      key.Data["keystorePassword"],
			"LIM_KEY_PASSWORD":           key.Data["keyPassword"],
			"LIM_PLAYSTORE_ACCESS_TOKEN": request.GoogleAccessToken,
		}
		updates, err = EnsureExpoIdentifier(request.ProjectPath, "android", "package", request.PackageName)
	} else {
		args = []string{"xcode", "build", request.ProjectPath, "--sdk", "iphoneos", "--configuration", "Release"}
		if certificate != nil {
			certificatePath, e := materialize(workDir, "certificate.p12", certificate.Data["certificateP12Base64"])
			if e != nil {
				return "", e
			}
			profilePath, e := materialize(workDir, "profile.mobileprovision", profile.Data["provisioningProfileBase64"])
			if e != nil {
				return "", e
			}
			args = append(args, "--certificate-p12", certificatePath, "--certificate-password", certificate.Data["certificatePassword"], "--provisioning-profile", profilePath)
		} else {
			args = append(args, "--signing-method", "app-store-connect", "--team-id", request.TeamID)
		}
		if kind == "publish" {
			keyPath, e := materialize(workDir, "AuthKey.p8", key.Data["privateKeyP8Base64"])
			if e != nil {
				return "", e
			}
			args = append(args, "--upload-to-appstore", "--auto-build-number", "--asc-key-id", key.Data["keyId"], "--asc-key", keyPath)
			if key.Data["issuerId"] != "" {
				args = append(args, "--asc-issuer-id", key.Data["issuerId"])
			}
		}
		if request.Scheme != "" {
			args = append(args, "--scheme", request.Scheme)
		}
		updates, err = EnsureExpoIdentifier(request.ProjectPath, "ios", "bundleIdentifier", request.BundleID)
	}
	if err != nil {
		return "", err
	}
	id, token, err := j.begin(request, kind)
	if err != nil {
		return "", err
	}
	header := "X-Publish-Token"
	if kind == "install" {
		header = "X-Install-Token"
		status, _ := j.Get(id)
		args = append(args, "--upload", status.AssetName)
	}
	args = append(args, "--webhook-url", request.WebhookURL, "--webhook-header", header+"="+token, "--inactivity-timeout", "3s", "--detach")
	if kind != "install" {
		args = append(args, "--json")
	}
	for _, line := range updates {
		log.Printf("[%s] %s", id, line)
	}
	log.Printf("[%s] starting detached %s build", id, kind)
	j.spawn(ctx, id, args, workDir, env, kind != "install")
	spawned = true
	return id, nil
}
