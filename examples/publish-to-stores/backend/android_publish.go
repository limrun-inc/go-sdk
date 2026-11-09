// Runs one Play Store publish with the same headless lifecycle as iOS:
// materialize the upload key, start `lim gradle build --detach` with a
// completion webhook, and let the frontend poll the callback state.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type androidPublishRequest struct {
	ProjectPath string `json:"projectPath"`
	PackageName string `json:"packageName"`
	// Google OAuth access token with the androidpublisher scope.
	GoogleAccessToken string `json:"googleAccessToken"`
	// Play track ID; defaults to internal.
	Track string `json:"track"`
	// Store directory the upload keystore lives in; the backend default when empty.
	SecretsDir string `json:"secretsDir"`
	// Public URL limbuild POSTs the build-finish webhook to, verbatim. It must
	// forward to this backend's webhook receiver for the result to be observed.
	WebhookURL string `json:"webhookUrl"`
}

func (r androidPublishRequest) validate() error {
	if r.ProjectPath == "" || r.WebhookURL == "" {
		return fmt.Errorf("projectPath and webhookUrl are required")
	}
	if r.PackageName == "" || r.GoogleAccessToken == "" {
		return fmt.Errorf("packageName and googleAccessToken are required")
	}
	return nil
}

// detectAndroidPackage finds the application ID with the lim CLI's heuristics:
// Expo app config first, then app/build.gradle(.kts) under the usual roots,
// skipping line comments so a commented-out ID cannot shadow the live one.
func detectAndroidPackage(root string) string {
	for _, path := range findExpoConfigs(root, 4) {
		var config struct {
			Expo struct {
				Android struct {
					Package string `json:"package"`
				} `json:"android"`
			} `json:"expo"`
		}
		data, err := os.ReadFile(path)
		if err == nil && json.Unmarshal(data, &config) == nil && config.Expo.Android.Package != "" {
			return config.Expo.Android.Package
		}
	}
	pattern := regexp.MustCompile(`applicationId\s*=?\s*["']([A-Za-z0-9_.]+)["']`)
	for _, subdir := range []string{".", "android"} {
		for _, file := range []string{"build.gradle", "build.gradle.kts"} {
			data, err := os.ReadFile(filepath.Join(root, subdir, "app", file))
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				code, _, _ := strings.Cut(line, "//")
				if match := pattern.FindStringSubmatch(code); match != nil {
					return match[1]
				}
			}
		}
	}
	return ""
}

// startAndroid materializes the upload keystore and spawns a detached
// `lim gradle build` that publishes the signed AAB to Google Play.
func (j *publishes) startAndroid(ctx context.Context, store *secretStore, request androidPublishRequest) (string, error) {
	// <package>/UPLOAD is the secret name convention for a package's Play
	// upload keystore; the frontend stores it under the same name.
	key, err := store.get("androidSigningKey", request.PackageName+"/UPLOAD", request.SecretsDir)
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
	workDir, err := os.MkdirTemp("", "limrun-android-")
	if err != nil {
		return "", err
	}
	spawned := false
	defer func() {
		if !spawned {
			os.RemoveAll(workDir)
		}
	}()
	keystorePath, err := materialize(workDir, "upload.p12", key.Data["keystoreBase64"])
	if err != nil {
		return "", err
	}
	track := request.Track
	if track == "" {
		track = "internal"
	}
	args := []string{
		"gradle", "build", request.ProjectPath,
		"--keystore", keystorePath,
		"--key-alias", key.Data["keyAlias"],
		"--upload-to-playstore",
		"--playstore-package", request.PackageName,
		"--playstore-track", track,
		"--auto-version-code",
	}
	// The passwords and Play token travel as environment variables so they
	// never appear in the process list.
	env := append(os.Environ(),
		"LIM_KEYSTORE_PASSWORD="+key.Data["keystorePassword"],
		"LIM_KEY_PASSWORD="+key.Data["keyPassword"],
		"LIM_PLAYSTORE_ACCESS_TOKEN="+request.GoogleAccessToken,
	)
	updates, err := ensureExpoIdentifier(request.ProjectPath, "android", "package", request.PackageName)
	if err != nil {
		return "", err
	}
	id, token, err := j.begin()
	if err != nil {
		return "", err
	}
	// --json turns the detach summary into one machine-readable object on stdout.
	args = append(args, "--webhook-url", request.WebhookURL, "--webhook-header", "X-Publish-Token="+token, "--inactivity-timeout", "3s", "--detach", "--json")
	for _, line := range updates {
		log.Printf("[%s] %s", id, line)
	}
	log.Printf("[%s] starting detached android build", id)
	j.spawn(ctx, id, args, workDir, env)
	spawned = true
	return id, nil
}
