package syncfolder

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeGitDir lays out the files a Git directory keeps. An empty remote leaves out the origin.
func writeGitDir(t *testing.T, gitDir, remote string) {
	t.Helper()
	config := "[core]\n\trepositoryformatversion = 0\n\tbare = false\n"
	if remote != "" {
		config += "[remote \"origin\"]\n\turl = " + strconv.Quote(remote) + "\n"
	}
	writeFixture(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
	writeFixture(t, filepath.Join(gitDir, "config"), config)
}

// addWorktree lays out what `git worktree add` leaves behind: a .git file pointing into the main checkout.
func addWorktree(t *testing.T, repo, worktree string, relativePointer bool) {
	t.Helper()
	gitDir := filepath.Join(repo, ".git", "worktrees", filepath.Base(worktree))
	writeFixture(t, filepath.Join(gitDir, "commondir"), "../..\n")
	writeFixture(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/feature\n")
	pointer := gitDir
	if relativePointer {
		var err error
		if pointer, err = filepath.Rel(worktree, gitDir); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture(t, filepath.Join(worktree, ".git"), "gitdir: "+pointer+"\n")
}

func TestIdentifyProject(t *testing.T) {
	type projectCase struct {
		name string
		// arrange changes the checkout and returns the project path to identify.
		arrange func(t *testing.T, repo string) string
		want    ProjectIdentity
		ok      bool
	}
	monorepo := ProjectIdentity{Repository: "github.com/acme/monorepo"}
	iosApp := ProjectIdentity{Repository: "github.com/acme/monorepo", Directory: "apps/ios"}
	root := func(_ *testing.T, repo string) string { return repo }
	withOrigin := func(remote string, project ...string) func(*testing.T, string) string {
		return func(t *testing.T, repo string) string {
			writeGitDir(t, filepath.Join(repo, ".git"), remote)
			dir := filepath.Join(append([]string{repo}, project...)...)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			return dir
		}
	}
	withConfig := func(config string) func(*testing.T, string) string {
		return func(t *testing.T, repo string) string {
			writeFixture(t, filepath.Join(repo, ".git", "config"), config)
			return repo
		}
	}
	withGitFile := func(pointer string) func(*testing.T, string) string {
		return func(t *testing.T, repo string) string {
			nested := filepath.Join(repo, "apps", "ios")
			writeFixture(t, filepath.Join(nested, ".git"), pointer)
			return nested
		}
	}
	withWorktree := func(relativePointer bool) func(*testing.T, string) string {
		return func(t *testing.T, repo string) string {
			worktree := filepath.Join(filepath.Dir(repo), "other-checkout")
			addWorktree(t, repo, worktree, relativePointer)
			if err := os.MkdirAll(filepath.Join(worktree, "apps", "ios"), 0755); err != nil {
				t.Fatal(err)
			}
			linked := filepath.Join(filepath.Dir(repo), "linked")
			if err := os.Symlink(worktree, linked); err != nil {
				t.Skip(err)
			}
			return filepath.Join(linked, "apps", "ios")
		}
	}
	cases := []projectCase{
		{name: "checkout root", arrange: root, want: monorepo, ok: true},
		{name: "project in a subdirectory", arrange: withOrigin("git@github.com:acme/monorepo.git", "apps", "ios"), want: iosApp, ok: true},
		{name: "outside Git", arrange: func(_ *testing.T, repo string) string { return filepath.Dir(repo) }},
		{name: "no origin", arrange: withOrigin("")},

		{name: "scp-like remote lowercases a GitHub path", arrange: withOrigin("git@github.com:Acme/Monorepo.git"), want: monorepo, ok: true},
		{name: "ssh URL", arrange: withOrigin("ssh://git@github.com/acme/monorepo.git", "apps", "ios"), want: iosApp, ok: true},
		{name: "https URL with a trailing slash", arrange: withOrigin("https://github.com/acme/monorepo.git/"), want: monorepo, ok: true},
		{name: "https URL without the .git suffix", arrange: withOrigin("https://GitHub.com/acme/monorepo"), want: monorepo, ok: true},
		{name: "https URL with the default port", arrange: withOrigin("https://github.com:443/acme/monorepo.git"), want: monorepo, ok: true},
		{name: "credentials and query are dropped", arrange: withOrigin("https://user:secret@github.com/acme/monorepo.git?token=secret"), want: monorepo, ok: true},
		{
			name:    "other hosts keep the port and the path case",
			arrange: withOrigin("ssh://git@Git.Example.com:2222/Acme/Mobile.git", "MyApp"),
			want:    ProjectIdentity{Repository: "git.example.com:2222/Acme/Mobile", Directory: "MyApp"},
			ok:      true,
		},
		{name: "local path remote", arrange: withOrigin("/tmp/local-repo")},
		{name: "file URL remote", arrange: withOrigin("file:///tmp/local-repo")},
		{name: "Windows drive remote", arrange: withOrigin(`C:\local\repo`)},
		{name: "scp-like remote with a parent segment", arrange: withOrigin("git@github.com:acme/../monorepo.git")},
		{name: "URL remote with a parent segment", arrange: withOrigin("https://github.com/acme/../monorepo.git")},
		{name: "remote without a repository path", arrange: withOrigin("https://github.com/")},

		{
			name: "quoted values, comments, continuations and case-insensitive names",
			arrange: withConfig(strings.Join([]string{
				`[core]`,
				`  editor = "tool \"arg\"" # unrelated escaped quotes`,
				`[ReMoTe "origin"] ; comment`,
				`  URL = "https://user:sec;ret@github.com/acme/\`,
				`monorepo.git" # origin`,
				`[remote "other"] url = https://github.com/acme/other.git`,
			}, "\r\n")),
			want: monorepo, ok: true,
		},
		{
			name: "first fetch URL and the legacy section spelling",
			arrange: withConfig(strings.Join([]string{
				`[REMOTE.ORIGIN]`,
				`url = https://github.com/acme/monorepo.git`,
				`url = https://github.com/acme/mirror.git`,
				`[remote "Origin"]`,
				`url = https://github.com/acme/uppercase-name.git`,
			}, "\n")),
			want: monorepo, ok: true,
		},
		{name: "config behind a byte order mark", arrange: withConfig("\xef\xbb\xbf[remote \"origin\"]\nurl = https://github.com/acme/monorepo.git\n"), want: monorepo, ok: true},
		{name: "unterminated quote", arrange: withConfig("[remote \"origin\"]\nurl = \"unterminated")},
		{name: "unknown escape", arrange: withConfig("[remote \"origin\"]\nurl = \"https://github.com/acme/\\q\"")},
		{name: "unterminated section", arrange: withConfig("[remote \"origin\"\nurl = https://github.com/acme/repo.git")},
		{name: "config with an include", arrange: withConfig("[remote \"origin\"]\nurl = https://github.com/acme/repo.git\n[include]\npath = other.config")},
		{name: "config with a conditional include", arrange: withConfig("[remote \"origin\"]\nurl = https://github.com/acme/repo.git\n[includeIf \"onbranch:main\"]\npath = other.config")},

		{
			name: "worktree config is ignored without the extension",
			arrange: func(t *testing.T, repo string) string {
				writeGitDir(t, filepath.Join(repo, ".git"), "")
				writeFixture(t, filepath.Join(repo, ".git", "config.worktree"), "[remote \"origin\"]\nurl = git@github.com:acme/worktree.git\n")
				return repo
			},
		},
		{
			name: "worktree config is read with the extension",
			arrange: func(t *testing.T, repo string) string {
				writeFixture(t, filepath.Join(repo, ".git", "config"), "[core]\n\tbare = false\n[extensions]\nworktreeConfig = true\n")
				writeFixture(t, filepath.Join(repo, ".git", "config.worktree"), "[remote \"origin\"]\nurl = git@github.com:acme/worktree.git\n")
				return repo
			},
			want: ProjectIdentity{Repository: "github.com/acme/worktree"}, ok: true,
		},

		{name: "worktree with an absolute pointer, reached through a symlink", arrange: withWorktree(false), want: iosApp, ok: true},
		{name: "worktree with a relative pointer, reached through a symlink", arrange: withWorktree(true), want: iosApp, ok: true},
		{
			name: "nested checkout uses the nearest repository",
			arrange: func(t *testing.T, repo string) string {
				nested := filepath.Join(repo, "apps", "ios")
				writeGitDir(t, filepath.Join(nested, ".git"), "git@github.com:acme/ios.git")
				return nested
			},
			want: ProjectIdentity{Repository: "github.com/acme/ios"}, ok: true,
		},
		{
			name: "submodule gitfile does not adopt the containing repository",
			arrange: func(t *testing.T, repo string) string {
				nested := filepath.Join(repo, "apps", "ios")
				writeGitDir(t, filepath.Join(repo, ".git", "modules", "apps", "ios"), "git@github.com:acme/ios.git")
				writeFixture(t, filepath.Join(nested, ".git"), "gitdir: ../../.git/modules/apps/ios\n")
				if err := os.MkdirAll(filepath.Join(nested, "Client"), 0755); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(nested, "Client")
			},
			want: ProjectIdentity{Repository: "github.com/acme/ios", Directory: "Client"}, ok: true,
		},
		{name: "gitfile without a pointer", arrange: withGitFile("not a gitfile")},
		{name: "gitfile pointing at a missing directory", arrange: withGitFile("gitdir: missing")},
		{name: "gitfile with an empty pointer", arrange: withGitFile("gitdir: ")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := filepath.Join(t.TempDir(), "local-checkout-name")
			writeGitDir(t, filepath.Join(repo, ".git"), "git@github.com:acme/monorepo.git")
			if err := os.MkdirAll(filepath.Join(repo, "apps", "ios"), 0755); err != nil {
				t.Fatal(err)
			}
			got, ok := IdentifyProject(tc.arrange(t, repo))
			if ok != tc.ok || got != tc.want {
				t.Errorf("identity=%+v ok=%v, want %+v ok=%v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestIdentifyProjectNeedsNoGitBinary(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "local-checkout-name")
	writeGitDir(t, filepath.Join(repo, ".git"), "git@github.com:acme/monorepo.git")
	t.Setenv("PATH", t.TempDir())
	got, ok := IdentifyProject(repo)
	if want := (ProjectIdentity{Repository: "github.com/acme/monorepo"}); !ok || got != want {
		t.Errorf("identity=%+v ok=%v", got, ok)
	}
}

func TestProjectFolderName(t *testing.T) {
	for _, tc := range []struct {
		name, remote, directory, want string
		ok                            bool
	}{
		{name: "repository root is named after the repository", remote: "git@github.com:acme/monorepo.git", want: "monorepo", ok: true},
		{name: "subdirectory keeps its name", remote: "git@github.com:acme/monorepo.git", directory: "apps/ios", want: "ios", ok: true},
		{name: "port in the host is not part of the name", remote: "ssh://git@git.example.com:2222/Acme/Mobile.git", want: "Mobile", ok: true},
		{name: "directory name the instance cannot use", remote: "git@github.com:acme/monorepo.git", directory: "My App"},
		{name: "repository name the instance cannot use", remote: "https://github.com/acme/bad%20name"},
		{name: "no origin", directory: "apps/ios"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := filepath.Join(t.TempDir(), "local-checkout-name")
			writeGitDir(t, filepath.Join(repo, ".git"), tc.remote)
			project := filepath.Join(repo, filepath.FromSlash(tc.directory))
			if err := os.MkdirAll(project, 0755); err != nil {
				t.Fatal(err)
			}
			got, ok := ProjectFolderName(project)
			if ok != tc.ok || (ok && got != tc.want) {
				t.Errorf("name=%q ok=%v, want %q ok=%v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
