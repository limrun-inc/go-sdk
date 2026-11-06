package syncfolder

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

type IgnoreOptions struct {
	BasisCacheDir       string
	XcodeDefaults       bool
	NestedGitignore     *bool
	Additional, Include func(string) bool
	Log                 func(string, string)
}
type ignoreRule struct {
	pattern       *regexp.Regexp
	negate        bool
	directoryOnly bool
	raw           string
}

func CreateIgnore(root string, opts IgnoreOptions) (func(string) bool, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	basis, err := filepath.Rel(root, opts.BasisCacheDir)
	if err != nil {
		return nil, err
	}
	basis = filepath.ToSlash(basis)
	ignoreBasis := basis != "" && basis != "." && basis != ".." && !strings.HasPrefix(basis, "../")
	nested := opts.XcodeDefaults
	if opts.NestedGitignore != nil {
		nested = *opts.NestedGitignore
	}
	tracked := map[string]bool{}
	command := exec.Command("git", "ls-files", "-z")
	command.Dir = root
	if out, err := command.Output(); err == nil {
		for _, file := range strings.Split(string(out), "\x00") {
			if file == "" {
				continue
			}
			tracked[file] = true
			for parent := path.Dir(file); parent != "."; parent = path.Dir(parent) {
				tracked[parent] = true
			}
		}
	}
	rules := map[string][]ignoreRule{}
	warned := map[string]bool{}
	var mu sync.Mutex
	load := func(dir string) []ignoreRule {
		if r, ok := rules[dir]; ok {
			return r
		}
		var r []ignoreRule
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), ".gitignore"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSuffix(line, "\r")
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if rule, ok := compileIgnoreRule(line); ok {
					r = append(r, rule)
				}
			}
		}
		rules[dir] = r
		return r
	}
	return func(relative string) bool {
		mu.Lock()
		defer mu.Unlock()
		normalized := filepath.ToSlash(relative)
		normalized = strings.TrimPrefix(normalized, "./")
		trimmed := strings.TrimRight(normalized, "/")
		if trimmed == "" {
			return false
		}
		if ignoreBasis && (trimmed == basis || strings.HasPrefix(trimmed, basis+"/")) {
			return true
		}
		if opts.Include != nil && opts.Include(normalized) {
			return false
		}
		if opts.Additional != nil && opts.Additional(normalized) {
			return true
		}
		parts := strings.Split(trimmed, "/")
		for _, part := range parts {
			if part == ".git" || part == ".DS_Store" {
				return true
			}
		}
		if opts.XcodeDefaults {
			for _, prefix := range []string{"build/", ".build/", "DerivedData/", "Index.noindex/", "ModuleCache.noindex/", ".index-build/", ".swiftpm/", "Pods/", "Carthage/Build/"} {
				if strings.HasPrefix(normalized, prefix) {
					return true
				}
			}
			if strings.Contains(normalized, "/xcuserdata/") || strings.Contains(normalized, ".dSYM/") {
				return true
			}
		}
		base := path.Base(trimmed)
		if strings.HasSuffix(trimmed, ".xcconfig") || strings.HasSuffix(base, ".env") || strings.HasPrefix(base, ".env.") {
			return false
		}
		depth := 1
		if nested {
			depth = len(parts)
		}
		ignored := false
		lastRule := ""
		suffix := ""
		if strings.HasSuffix(normalized, "/") {
			suffix = "/"
		}
		for i := 0; i < depth; i++ {
			dir := strings.Join(parts[:i], "/")
			candidate := strings.Join(parts[i:], "/") + suffix
			if value, matched, rule := matchIgnoreLayer(load(dir), candidate); matched {
				ignored, lastRule = value, rule
			}
		}
		if ignored && tracked[trimmed] && !warned[lastRule] {
			warned[lastRule] = true
			if opts.Log != nil {
				opts.Log("warn", fmt.Sprintf(".gitignore rule %q is dropping %q, which is tracked in git. The remote build will not see this path.", lastRule, trimmed))
			}
		}
		return ignored
	}, nil
}

// A negation cannot rescue a child of an excluded directory in the same .gitignore.
func matchIgnoreLayer(rules []ignoreRule, candidate string) (ignored, matched bool, lastRule string) {
	parts := strings.Split(strings.TrimSuffix(candidate, "/"), "/")
	for i := 1; i <= len(parts); i++ {
		path := strings.Join(parts[:i], "/")
		if i < len(parts) || strings.HasSuffix(candidate, "/") {
			path += "/"
		}
		ignored, matched, lastRule = false, false, ""
		for _, rule := range rules {
			candidate := path
			if !rule.directoryOnly {
				candidate = strings.TrimSuffix(candidate, "/")
			}
			if rule.pattern.MatchString(candidate) {
				ignored, matched, lastRule = !rule.negate, true, rule.raw
			}
		}
		if ignored {
			return
		}
	}
	return
}

// Compile git's path globs rather than filesystem globs: rules without slashes match at any depth.
func compileIgnoreRule(raw string) (ignoreRule, bool) {
	line := raw
	for strings.HasSuffix(line, " ") {
		slashes := 0
		for i := len(line) - 2; i >= 0 && line[i] == '\\'; i-- {
			slashes++
		}
		if slashes%2 == 1 {
			break
		}
		line = line[:len(line)-1]
	}
	if line == "" || line[0] == '#' {
		return ignoreRule{}, false
	}
	rule := ignoreRule{raw: raw}
	if line[0] == '!' {
		rule.negate = true
		line = line[1:]
	}
	if line == "" {
		return ignoreRule{}, false
	}
	rule.directoryOnly = strings.HasSuffix(line, "/")
	if rule.directoryOnly {
		line = strings.TrimSuffix(line, "/")
	}
	anchored := strings.Contains(line, "/")
	line = strings.TrimPrefix(line, "/")
	var pattern strings.Builder
	if anchored {
		pattern.WriteString("^")
	} else {
		pattern.WriteString("(?:^|/)")
	}
	for i := 0; i < len(line); {
		ch := line[i]
		switch ch {
		case '\\':
			i++
			if i == len(line) {
				return ignoreRule{}, false
			}
			pattern.WriteString(regexp.QuoteMeta(line[i : i+1]))
			i++
		case '*':
			end := i + 1
			for end < len(line) && line[end] == '*' {
				end++
			}
			if end-i >= 2 && (i == 0 || line[i-1] == '/') && (end == len(line) || line[end] == '/') {
				if end < len(line) {
					pattern.WriteString("(?:[^/]+/)*")
					end++
				} else {
					pattern.WriteString(".*")
				}
			} else {
				pattern.WriteString("[^/]*")
			}
			i = end
		case '?':
			pattern.WriteString("[^/]")
			i++
		case '[':
			end := i + 1
			if end < len(line) && (line[end] == '!' || line[end] == '^') {
				end++
			}
			if end < len(line) && line[end] == ']' {
				end++
			}
			for end < len(line) && line[end] != ']' {
				if line[end] == '\\' {
					end++
				}
				end++
			}
			if end >= len(line) {
				pattern.WriteString(`\[`)
				i++
				continue
			}
			class := line[i+1 : end]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			} else if strings.HasPrefix(class, "^") {
				class = `\^` + class[1:]
			}
			if strings.HasPrefix(class, "]") {
				class = `\]` + class[1:]
			}
			fragment := "[" + class + "]"
			if _, err := regexp.Compile(fragment); err != nil {
				pattern.WriteString(regexp.QuoteMeta(line[i : end+1]))
			} else {
				pattern.WriteString(fragment)
			}
			i = end + 1
		default:
			pattern.WriteString(regexp.QuoteMeta(line[i : i+1]))
			i++
		}
	}
	if rule.directoryOnly {
		pattern.WriteString("/")
	}
	pattern.WriteString("$")
	compiled, err := regexp.Compile(pattern.String())
	if err != nil {
		return ignoreRule{}, false
	}
	rule.pattern = compiled
	return rule, true
}
