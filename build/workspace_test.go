package build

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWorkspaceMatchesTypeScript(t *testing.T) {
	for _, flag := range []bool{false, true} {
		want, err := os.ReadFile("testdata/xcode-" + strconv.FormatBool(flag) + ".BUILD")
		if err != nil {
			t.Fatal(err)
		}
		got, err := RenderXcodeConfigBuild("26.4.0.17E192", flag)
		if err != nil || got != string(want) {
			t.Fatalf("BUILD loads=%v: %v\n%s", flag, err, got)
		}
		want, err = os.ReadFile("testdata/bazelrc-" + strconv.FormatBool(flag))
		if err != nil {
			t.Fatal(err)
		}
		got, err = RenderLimrunBazelrc(8980, "26.4.0.17E192", flag)
		if err != nil || got != string(want) {
			t.Fatalf("bazelrc mac=%v: %v\n%s", flag, err, got)
		}
	}
}
func TestWorkspaceDiscoveryAndWiring(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "MODULE.bazel"), nil, 0644)
	app := filepath.Join(root, "App")
	os.Mkdir(app, 0755)
	os.WriteFile(filepath.Join(app, "BUILD"), []byte("# ios_application(\n# name = \"fake\"\nios_application(\n    name = \"App\",\n)\n"), 0644)
	got, err := FindBazelWorkspaceRoot(app)
	if err != nil || got != root {
		t.Fatal(got, err)
	}
	if got = InferBuildTarget(root); got != "//App" {
		t.Fatal(got)
	}
	os.WriteFile(filepath.Join(root, ".bazelrc"), []byte("# "+TryImportLine+"\n"), 0644)
	changed, err := EnsureTryImport(root)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	changed, err = EnsureTryImport(root)
	if err != nil || changed {
		t.Fatal(changed, err)
	}
	data, _ := os.ReadFile(filepath.Join(root, ".bazelrc"))
	if strings.Count(string(data), TryImportLine) != 2 {
		t.Fatal(string(data))
	}
}
