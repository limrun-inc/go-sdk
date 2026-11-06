package build

import (
	_ "embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const LimrunDir = ".limrun"
const TryImportLine = "try-import %workspace%/.limrun/bazelrc"
const tryImportComment = "# Added by Limrun remote-execution setup: loads the generated config."

//go:embed templates/xcode.BUILD
var xcodeBuildTemplate string

//go:embed templates/bazelrc
var bazelrcTemplate string

func FindBazelWorkspaceRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		for _, marker := range []string{"MODULE.bazel", "WORKSPACE", "WORKSPACE.bazel"} {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}
func InferBuildTarget(root string) string {
	var found []string
	appRule := regexp.MustCompile(`\b(?:ios|macos|tvos|watchos)_application\s*\(`)
	targetName := regexp.MustCompile(`^\s*name\s*=\s*"([^"]+)"`)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(found) > 1 {
			return fs.SkipAll
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == LimrunDir || strings.HasPrefix(d.Name(), "bazel-") {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != "BUILD" && d.Name() != "BUILD.bazel" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		pkg, _ := filepath.Rel(root, filepath.Dir(path))
		pkg = filepath.ToSlash(pkg)
		inRule := false
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimLeft(line, " \t\r"), "#") {
				continue
			}
			if !inRule {
				inRule = appRule.MatchString(line)
				continue
			}
			match := targetName.FindStringSubmatch(line)
			if len(match) > 1 {
				name := match[1]
				label := "//" + pkg + ":" + name
				if pkg == "" || pkg == "." {
					label = "//:" + name
				} else if filepath.Base(pkg) == name {
					label = "//" + pkg
				}
				found = append(found, label)
				inRule = false
			} else if strings.HasPrefix(line, ")") {
				inRule = false
			}
		}
		return nil
	})
	if len(found) == 1 {
		return found[0]
	}
	return ""
}
func DetectBazelMajorVersion(dir string) *int {
	raw, err := os.ReadFile(filepath.Join(dir, ".bazelversion"))
	if err != nil {
		return nil
	}
	first := strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
	match := regexp.MustCompile(`^\d+`).FindString(first)
	if match == "" {
		return nil
	}
	n, err := strconv.Atoi(match)
	if err != nil {
		return nil
	}
	return &n
}
func IsBazel9OrLater(major *int) bool { return major == nil || *major >= 9 }
func shortXcodeVersion(key string) (string, error) {
	parts := strings.Split(key, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("unexpected Xcode version key from the instance: %s", key)
	}
	return strings.Join(parts[:2], "."), nil
}
func RenderXcodeConfigBuild(key string, emitLoads bool) (string, error) {
	short, err := shortXcodeVersion(key)
	if err != nil {
		return "", err
	}
	parts := strings.Split(key, ".")
	fullAlias := strings.Join(parts[:3], ".")
	loads := ""
	if emitLoads {
		loads = "load(\"@build_bazel_apple_support//xcode:xcode_version.bzl\", \"xcode_version\")\nload(\"@build_bazel_apple_support//xcode:available_xcodes.bzl\", \"available_xcodes\")\nload(\"@build_bazel_apple_support//xcode:xcode_config.bzl\", \"xcode_config\")\n\n"
	}
	return strings.NewReplacer("MAJOR.MINOR.PATCH.BUILD", key, "MAJOR.MINOR.PATCH", fullAlias, "MAJOR.MINOR", short, "{{LOADS}}", loads).Replace(xcodeBuildTemplate), nil
}
func RenderLimrunBazelrc(port int, key string, isMacClient bool) (string, error) {
	short, err := shortXcodeVersion(key)
	if err != nil {
		return "", err
	}
	platform := ""
	if !isMacClient {
		platform = "build:limrun --extra_execution_platforms=@build_bazel_apple_support//platforms:darwin_arm64\n"
	}
	return strings.NewReplacer("{{PORT}}", strconv.Itoa(port), "MAJOR.MINOR", short, "{{EXEC_PLATFORM}}", platform).Replace(bazelrcTemplate), nil
}
func EnsureTryImport(dir string) (bool, error) {
	path := filepath.Join(dir, ".bazelrc")
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	current := string(raw)
	for _, line := range strings.Split(current, "\n") {
		if strings.TrimSpace(line) == TryImportLine {
			return false, nil
		}
	}
	next := tryImportComment + "\n" + TryImportLine + "\n"
	if current != "" {
		next = strings.TrimRight(current, "\n") + "\n\n" + next
	}
	if err = os.WriteFile(path, []byte(next), 0644); err != nil {
		return false, err
	}
	return true, nil
}

type RBEWorkspaceOptions struct {
	IsMacClient *bool
	BazelMajor  *int
}
type RBEWorkspaceFiles struct {
	BuildFile, BazelrcFragment string
	BazelrcUpdated             bool
}

func WriteRBEWorkspaceFiles(workspace, key string, port int, options ...RBEWorkspaceOptions) (*RBEWorkspaceFiles, error) {
	var opts RBEWorkspaceOptions
	if len(options) > 0 {
		opts = options[0]
	}
	mac := runtime.GOOS == "darwin"
	if opts.IsMacClient != nil {
		mac = *opts.IsMacClient
	}
	major := opts.BazelMajor
	if major == nil {
		major = DetectBazelMajorVersion(workspace)
	}
	build, err := RenderXcodeConfigBuild(key, IsBazel9OrLater(major))
	if err != nil {
		return nil, err
	}
	rc, err := RenderLimrunBazelrc(port, key, mac)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(workspace, LimrunDir)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	result := &RBEWorkspaceFiles{BuildFile: filepath.Join(dir, "BUILD"), BazelrcFragment: filepath.Join(dir, "bazelrc")}
	for _, file := range []struct{ path, text string }{{result.BuildFile, build}, {result.BazelrcFragment, rc}, {filepath.Join(dir, ".gitignore"), "*\n"}} {
		if err = os.WriteFile(file.path, []byte(file.text), 0644); err != nil {
			return nil, err
		}
	}
	result.BazelrcUpdated, err = EnsureTryImport(workspace)
	return result, err
}
