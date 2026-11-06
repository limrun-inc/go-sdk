package syncfolder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIgnoreMatchesTypeScriptFixtures(t *testing.T) {
	var fixture struct {
		Files map[string]string `json:"files"`
		Cases []struct {
			Options struct {
				XcodeDefaults                   bool  `json:"xcodeDefaults"`
				NestedGitignore                 *bool `json:"nestedGitignore"`
				IncludePrefixes, IgnoreSuffixes []string
			}
			Paths []struct {
				Path    string
				Ignored bool
			}
		}
	}
	data, err := os.ReadFile("testdata/ignore.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for name, content := range fixture.Files {
		p := filepath.Join(root, name)
		if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, fixtureCase := range fixture.Cases {
		options := fixtureCase.Options
		fn, err := CreateIgnore(root, IgnoreOptions{BasisCacheDir: filepath.Join(root, ".cache"), XcodeDefaults: options.XcodeDefaults, NestedGitignore: options.NestedGitignore, Include: func(p string) bool {
			for _, prefix := range options.IncludePrefixes {
				if strings.HasPrefix(p, prefix) {
					return true
				}
			}
			return false
		}, Additional: func(p string) bool {
			for _, suffix := range options.IgnoreSuffixes {
				if strings.HasSuffix(p, suffix) {
					return true
				}
			}
			return false
		}})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range fixtureCase.Paths {
			if got := fn(item.Path); got != item.Ignored {
				t.Errorf("options=%+v path=%q ignored=%v, TS=%v", options, item.Path, got, item.Ignored)
			}
		}
	}
}
