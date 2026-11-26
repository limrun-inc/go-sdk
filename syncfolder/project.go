package syncfolder

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unicode"
)

// ProjectIdentity is what a project is, wherever it is checked out.
type ProjectIdentity struct {
	// Repository is the origin host and repository path, without credentials or the .git suffix.
	Repository string
	// Directory is the path from the checkout root to the project, with forward slashes. Empty at the root.
	Directory string
}

// IdentifyProject identifies a project from its Git checkout, so clones and worktrees of one
// repository agree. It reads repository-local files only and never runs Git. It reports false
// outside a checkout, without an origin, or when the metadata cannot be read with confidence.
func IdentifyProject(projectPath string) (ProjectIdentity, bool) {
	identity, err := identifyProject(projectPath)
	// Missing or unsupported metadata leaves the project unidentified.
	return identity, err == nil
}

// ProjectFolderName names the folder a project syncs into. A checkout at the repository root is
// named after the repository, not after the local folder, so every clone and worktree lands in
// the same place and reuses the same build state. It reports false when the project cannot be
// identified.
func ProjectFolderName(projectPath string) (string, bool) {
	project, ok := IdentifyProject(projectPath)
	if !ok {
		return "", false
	}
	name := project.Directory
	if name == "" {
		name = project.Repository
	}
	// Only a plain name is used as a folder. Anything else leaves the choice to the caller.
	if name = path.Base(name); !folderNamePattern.MatchString(name) {
		return "", false
	}
	return name, true
}

// The patterns below mirror the TypeScript SDK, so both SDKs name the same folder. JavaScript's
// "." and "\s" differ from Go's, hence the explicit classes.
const (
	notLineEnd = `[^\n\r\x{2028}\x{2029}]`
	jsSpace    = `\s\x0b\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`
)

var (
	errNotIdentified  = errors.New("project not identified")
	folderNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._+-]*$`)
	gitFilePattern    = regexp.MustCompile(`^gitdir: (` + notLineEnd + `+)$`)
	sectionPattern    = regexp.MustCompile(`^\[([\w.-]+)(?:[ \t]+"((?:[^"\\]|\\` + notLineEnd + `)*)")?\](` + notLineEnd + `*)$`)
	sectionEscape     = regexp.MustCompile(`\\(` + notLineEnd + `)`)
	settingPattern    = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*)(?:[ \t]*=[ \t]*(` + notLineEnd + `*))?$`)
	scpRemotePattern  = regexp.MustCompile(`^(?:[^/@:` + jsSpace + `]+@)?([^/:` + jsSpace + `]+):(` + notLineEnd + `+)$`)
	drivePathPattern  = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
)

func identifyProject(projectPath string) (ProjectIdentity, error) {
	none := ProjectIdentity{}
	absolute, err := filepath.Abs(projectPath)
	if err != nil {
		return none, err
	}
	cwd, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return none, err
	}
	root, gitDir, err := findCheckout(cwd)
	if err != nil {
		return none, err
	}
	commonDir := gitDir
	common, err := readOptional(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return none, err
	}
	if common = trimSpace(common); common != "" {
		commonDir = resolvePath(gitDir, common)
	}
	config, err := readConfig(filepath.Join(commonDir, "config"))
	if err != nil {
		return none, err
	}
	if enabled := config["extensions.worktreeconfig"]; len(enabled) > 0 {
		switch strings.ToLower(enabled[len(enabled)-1]) {
		case "true", "yes", "on", "1":
			worktree, err := readConfig(filepath.Join(gitDir, "config.worktree"))
			if err != nil {
				return none, err
			}
			for name, values := range worktree {
				config[name] = append(config[name], values...)
			}
		}
	}
	// Git fetch uses the first URL when a remote declares more than one.
	origins := config["remote.origin.url"]
	if len(origins) == 0 || origins[0] == "" {
		return none, errNotIdentified
	}
	repository, ok := repositoryIdentity(origins[0])
	if !ok {
		return none, errNotIdentified
	}
	directory, err := filepath.Rel(root, cwd)
	if err != nil {
		return none, err
	}
	directory = filepath.ToSlash(directory)
	if directory == "." {
		directory = ""
	}
	if directory == ".." || strings.HasPrefix(directory, "../") {
		return none, errNotIdentified
	}
	return ProjectIdentity{Repository: repository, Directory: directory}, nil
}

func findCheckout(start string) (root, gitDir string, err error) {
	for root = start; ; root = filepath.Dir(root) {
		marker := filepath.Join(root, ".git")
		if _, err = os.Lstat(marker); err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
			return "", "", err
		}
		if filepath.Dir(root) == root {
			return "", "", errNotIdentified
		}
	}
	marker := filepath.Join(root, ".git")
	info, err := os.Stat(marker)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() {
		return root, marker, nil
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		return "", "", err
	}
	gitFile := gitFilePattern.FindStringSubmatch(trimSpace(string(data)))
	// A broken nested checkout must not inherit the parent repository's identity.
	if gitFile == nil {
		return "", "", errNotIdentified
	}
	return root, resolvePath(root, gitFile[1]), nil
}

func resolvePath(base, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Join(base, target)
}

// readOptional returns an empty string for a missing file, which Git also treats as empty.
func readOptional(name string) (string, error) {
	data, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}

func readConfig(name string) (map[string][]string, error) {
	text, err := readOptional(name)
	if err != nil {
		return nil, err
	}
	return parseConfig(text)
}

// trimSpace trims what JavaScript's trim does. That includes a byte order mark, which
// Windows editors leave at the start of a config file.
func trimSpace(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return r == '\ufeff' || (unicode.IsSpace(r) && r != '\u0085')
	})
}

// parseConfig reads repository-local settings without inheriting machine-specific Git configuration.
func parseConfig(text string) (map[string][]string, error) {
	lines, err := configLines(text)
	if err != nil {
		return nil, err
	}
	config := map[string][]string{}
	section := ""
	for _, line := range lines {
		line = trimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			header := sectionPattern.FindStringSubmatchIndex(line)
			if header == nil {
				return nil, errors.New("invalid Git config section")
			}
			section = strings.ToLower(line[header[2]:header[3]])
			// Section names ignore case. Subsection names, such as a remote's, do not.
			if header[4] >= 0 {
				section += "." + sectionEscape.ReplaceAllString(line[header[4]:header[5]], "$1")
			}
			line = trimSpace(line[header[6]:header[7]])
			if line == "" {
				continue
			}
		}
		setting := settingPattern.FindStringSubmatchIndex(line)
		if section == "" || setting == nil {
			return nil, errors.New("invalid Git config setting")
		}
		key := strings.ToLower(line[setting[2]:setting[3]])
		name := section + "." + key
		// Includes can replace the origin. Refuse them instead of guessing from a partial config.
		if name == "include.path" || (strings.HasPrefix(section, "includeif.") && key == "path") {
			return nil, errors.New("includes in Git config are not followed")
		}
		// A setting without a value is a boolean true.
		value := "true"
		if setting[4] >= 0 {
			if value, err = configValue(line[setting[4]:setting[5]]); err != nil {
				return nil, err
			}
		}
		config[name] = append(config[name], value)
	}
	return config, nil
}

// configLines splits a config into logical lines. Comments and continuations depend on
// quoting, so splitting at '#' or newlines alone loses URLs.
func configLines(text string) ([]string, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := []string{}
	var line strings.Builder
	quoted, comment := false, false
	for i := 0; i < len(text); i++ {
		char := text[i]
		switch {
		case char == '\n':
			if quoted {
				return nil, errors.New("unterminated Git config quote")
			}
			lines = append(lines, line.String())
			line.Reset()
			comment = false
		case comment:
		case char == '\\':
			i++
			if i == len(text) {
				return nil, errors.New("incomplete Git config escape")
			}
			if text[i] != '\n' {
				line.WriteByte(char)
				line.WriteByte(text[i])
			}
		case !quoted && (char == '#' || char == ';'):
			comment = true
		default:
			if char == '"' {
				quoted = !quoted
			}
			line.WriteByte(char)
		}
	}
	if quoted {
		return nil, errors.New("unterminated Git config quote")
	}
	return append(lines, line.String()), nil
}

func configValue(raw string) (string, error) {
	escapes := map[byte]byte{'n': '\n', 't': '\t', 'b': '\b', '\\': '\\', '"': '"'}
	var value strings.Builder
	for i := 0; i < len(raw); i++ {
		char := raw[i]
		if char == '\\' {
			i++
			if i == len(raw) {
				return "", errors.New("invalid Git config escape")
			}
			escaped, ok := escapes[raw[i]]
			if !ok {
				return "", errors.New("invalid Git config escape")
			}
			value.WriteByte(escaped)
		} else if char != '"' {
			value.WriteByte(char)
		}
	}
	return value.String(), nil
}

// repositoryIdentity reduces a remote to host and path, so the transport and the
// credentials a checkout happens to use do not change the identity.
func repositoryIdentity(remote string) (string, bool) {
	var host, repo string
	if strings.Contains(remote, "://") {
		// A backslash means different things to different URL parsers, so it is not guessed at.
		if strings.Contains(remote, `\`) {
			return "", false
		}
		// The TypeScript SDK's URL parser drops tabs, newlines and surrounding spaces. Match it,
		// so a stray space in a quoted URL does not change the identity.
		remote = strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(remote)
		u, err := url.Parse(strings.TrimFunc(remote, func(r rune) bool { return r <= ' ' }))
		if err != nil {
			return "", false
		}
		defaultPort, supported := map[string]string{"https": ":443", "http": ":80", "ssh": ":", "git": ":"}[u.Scheme]
		if !supported {
			return "", false
		}
		// An explicit default port names the same server as no port.
		host = strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(u.Host), defaultPort), ":")
		if host == "" {
			return "", false
		}
		repo = u.EscapedPath()
		// URL parsers disagree on whether dot segments resolve, encoded ones too, so they are refused.
		for _, part := range strings.Split(repo, "/") {
			if decoded, err := url.PathUnescape(part); err == nil && (decoded == "." || decoded == "..") {
				return "", false
			}
		}
	} else {
		scp := scpRemotePattern.FindStringSubmatch(remote)
		if scp == nil || drivePathPattern.MatchString(remote) {
			return "", false
		}
		host, repo = strings.ToLower(scp[1]), scp[2]
	}
	repo = strings.TrimSuffix(strings.Trim(repo, "/"), ".git")
	if repo == "" {
		return "", false
	}
	for _, part := range strings.Split(repo, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	// GitHub repository names are case-insensitive; other hosts may preserve case.
	if host == "github.com" {
		repo = strings.ToLower(repo)
	}
	return host + "/" + repo, true
}
