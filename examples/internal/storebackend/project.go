package storebackend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func FindExpoConfigs(root string, depth int) []string {
	var found []string
	entries, err := os.ReadDir(root)
	if err != nil {
		return found
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if !entry.IsDir() && entry.Name() == "app.json" {
			if data, err := os.ReadFile(path); err == nil {
				var config map[string]any
				if json.Unmarshal(data, &config) == nil && config["expo"] != nil {
					found = append(found, path)
				}
			}
		} else if entry.IsDir() && depth > 0 {
			found = append(found, FindExpoConfigs(path, depth-1)...)
		}
	}
	return found
}

// Like the TS examples, fill only a missing Expo identifier; preserve existing values.
func EnsureExpoIdentifier(root, platform, key, value string) ([]string, error) {
	files := FindExpoConfigs(root, 4)
	if len(files) == 0 {
		return nil, nil
	}
	if len(files) > 1 {
		return []string{fmt.Sprintf("Multiple Expo app.json files: set expo.%s.%s to %s manually.", platform, key, value)}, nil
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	expo, ok := config["expo"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expo must be an object in %s", files[0])
	}
	section, _ := expo[platform].(map[string]any)
	if section == nil {
		section = map[string]any{}
		expo[platform] = section
	}
	if existing, ok := section[key].(string); ok && existing != "" {
		if existing != value {
			return []string{
				fmt.Sprintf("%s declares %s, but the build targets %s; leaving the existing value unchanged.", files[0], existing, value),
			}, nil
		}
		return nil, nil
	}
	section[key] = value
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(files[0], append(encoded, '\n'), 0644); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("Set expo.%s.%s in %s to %s.", platform, key, files[0], value)}, nil
}

func DetectAndroidPackage(root string) string {
	for _, path := range FindExpoConfigs(root, 4) {
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
