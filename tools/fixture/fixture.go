package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
)

// entry is one file in the Fixture. Its path is slash-separated and relative
// to the directory the Fixture is built in, and its parents are made for it.
type entry struct {
	path    string
	content content
}

type content interface {
	create(dir, target string) error
}

type file string

// hardLink is the path of an earlier entry that this one shares a file with.
type hardLink string

func (f file) create(_, target string) error {
	return os.WriteFile(target, []byte(f), 0o644)
}

func (l hardLink) create(dir, target string) error {
	return os.Link(filepath.Join(dir, filepath.FromSlash(string(l))), target)
}

// link is the slash path, from the Fixture root, that this entry points at.
type link string

func (l link) create(dir, target string) error {
	to := filepath.Join(dir, filepath.FromSlash(string(l)))
	rel, err := filepath.Rel(filepath.Dir(target), to)
	if err != nil {
		return err
	}
	err = os.Symlink(rel, target)
	if err == nil || runtime.GOOS != "windows" {
		return err
	}
	// A Windows account cannot always make a symlink. A junction is a Link
	// and needs no privilege.
	out, jerr := exec.Command("cmd", "/c", "mklink", "/J", target, to).CombinedOutput()
	if jerr != nil {
		return fmt.Errorf("mklink /J: %w: %s", jerr, out)
	}
	return nil
}

// committed is written before the repository commits, and git tracks it.
var committed = []entry{
	{"projects/web-app/package.json", manifest("web-app")},
	{"projects/web-app/src/index.js", file(`console.log(require("fake-alpha"), require("fake-beta"));` + "\n")},
	{"projects/web-app/node_modules/fake-alpha/package.json", manifest("fake-alpha")},
	{"projects/web-app/node_modules/fake-alpha/index.js", module("fake-alpha")},
	{"projects/web-app/node_modules/fake-alpha/LICENSE", license()},
	{"projects/web-app/node_modules/fake-beta/package.json", manifest("fake-beta")},
	{"projects/web-app/node_modules/fake-beta/index.js", module("fake-beta")},
	{"projects/web-app/node_modules/fake-beta/LICENSE", hardLink("projects/web-app/node_modules/fake-alpha/LICENSE")},
	{"projects/web-app/node_modules/fake-alpha/node_modules/fake-epsilon/package.json", manifest("fake-epsilon")},
	{"projects/web-app/node_modules/fake-alpha/node_modules/fake-epsilon/index.js", module("fake-epsilon")},

	{"projects/stale-app/package.json", manifest("stale-app")},
	{"projects/stale-app/src/index.js", file("console.log(\"stale\");\n")},
	{"projects/stale-app/node_modules/fake-old/package.json", manifest("fake-old")},
	{"projects/stale-app/node_modules/fake-old/index.js", module("fake-old")},
	{"projects/stale-app/node_modules/fake-old/note.txt", file("old\n")},
	{"projects/stale-app/node_modules/fake-old/note-again.txt", hardLink("projects/stale-app/node_modules/fake-old/note.txt")},

	{"projects/recent-app/package.json", manifest("recent-app")},
	{"projects/recent-app/src/index.js", file("console.log(\"recent\");\n")},
	{"projects/recent-app/node_modules/fake-new/package.json", manifest("fake-new")},
	{"projects/recent-app/node_modules/fake-new/index.js", module("fake-new")},
	{"projects/recent-app/node_modules/fake-new/note.txt", file("new\n")},
	{"projects/recent-app/node_modules/fake-new/note-again.txt", hardLink("projects/recent-app/node_modules/fake-new/note.txt")},

	{"projects/shifted-app/package.json", manifest("shifted-app")},
	{"projects/shifted-app/src/index.js", file("console.log(\"shifted\");\n")},
	{"projects/shifted-app/node_modules/fake-shift/package.json", manifest("fake-shift")},
	{"projects/shifted-app/node_modules/fake-shift/index.js", module("fake-shift")},
	{"projects/shifted-app/node_modules/fake-shift/note.txt", file("shift\n")},
	{"projects/shifted-app/node_modules/fake-shift/note-again.txt", hardLink("projects/shifted-app/node_modules/fake-shift/note.txt")},

	{"projects/tracked-app/package.json", manifest("tracked-app")},
	{"projects/tracked-app/node_modules/fake-gamma/package.json", manifest("fake-gamma")},
	{"projects/tracked-app/node_modules/fake-gamma/index.js", module("fake-gamma")},
	{"projects/tracked-app/.gitignore", file("ignored-app/node_modules/\n")},
	{"projects/tracked-app/ignored-app/package.json", manifest("ignored-app")},
	{"projects/tracked-app/untracked-app/package.json", manifest("untracked-app")},

	{"outside/stray-app/package.json", manifest("stray-app")},
	{"outside/stray-app/node_modules/fake-delta/package.json", manifest("fake-delta")},
	{"outside/stray-app/node_modules/fake-delta/index.js", module("fake-delta")},

	{"outside/stash-app/package.json", manifest("stash-app")},
	{"outside/stash-app/node_modules/fake-theta/package.json", manifest("fake-theta")},
	{"outside/stash-app/node_modules/fake-theta/index.js", module("fake-theta")},
	{"projects/linked-app/package.json", manifest("linked-app")},
	{"projects/linked-app/through", link("outside/stash-app")},

	{"projects/rust-app/Cargo.toml", file("[package]\nname = \"rust-app\"\nversion = \"0.1.0\"\nedition = \"2021\"\n")},
	{"projects/rust-app/src/main.rs", file("fn main() {}\n")},
	{"projects/rust-app/target/debug/deps/lib.rlib", file("synthetic build output\n")},
	{"projects/rust-app/target/debug/deps/lib-again.rlib", hardLink("projects/rust-app/target/debug/deps/lib.rlib")},

	{"projects/plain/target/note.txt", file("not a crate\n")},
}

func npmCachePath() string {
	if runtime.GOOS == "windows" {
		return "home/AppData/Local/npm-cache/_cacache"
	}
	return "home/.npm/_cacache"
}

func npmCache() []entry {
	cache := npmCachePath()
	return []entry{
		{cache + "/index-v5/a", file("synthetic npm cache\n")},
		{cache + "/index-v5/b", hardLink(cache + "/index-v5/a")},
	}
}

// ignored is written before the commit. The repository's gitignore names it,
// so git add leaves it out and the Git rule does not keep it.
var ignored = []entry{
	{"projects/tracked-app/ignored-app/node_modules/fake-zeta/package.json", manifest("fake-zeta")},
	{"projects/tracked-app/ignored-app/node_modules/fake-zeta/index.js", module("fake-zeta")},
	{"projects/tracked-app/ignored-app/node_modules/fake-zeta/note.txt", file("zeta\n")},
	{"projects/tracked-app/ignored-app/node_modules/fake-zeta/note-again.txt", hardLink("projects/tracked-app/ignored-app/node_modules/fake-zeta/note.txt")},
}

// untracked is written after the commit, and no gitignore names it.
var untracked = []entry{
	{"projects/tracked-app/untracked-app/node_modules/fake-eta/package.json", manifest("fake-eta")},
	{"projects/tracked-app/untracked-app/node_modules/fake-eta/index.js", module("fake-eta")},
	{"projects/tracked-app/untracked-app/node_modules/fake-eta/note.txt", file("eta\n")},
	{"projects/tracked-app/untracked-app/node_modules/fake-eta/note-again.txt", hardLink("projects/tracked-app/untracked-app/node_modules/fake-eta/note.txt")},
}

var repositories = []string{"projects/tracked-app"}

type answerKey struct {
	Version    int         `json:"version"`
	Root       string      `json:"root"`
	Candidates []candidate `json:"candidates"`
	Traps      []trap      `json:"traps"`
}

type candidate struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type trap struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

var key = answerKey{
	Version: 1,
	Root:    "projects",
	Candidates: []candidate{
		{Path: "projects/web-app/node_modules", Kind: domain.Dependencies.String()},
		{Path: "projects/stale-app/node_modules", Kind: domain.Dependencies.String()},
		{Path: "projects/recent-app/node_modules", Kind: domain.Dependencies.String()},
		{Path: "projects/shifted-app/node_modules", Kind: domain.Dependencies.String()},
		{Path: "projects/tracked-app/ignored-app/node_modules", Kind: domain.Dependencies.String()},
		{Path: "projects/tracked-app/untracked-app/node_modules", Kind: domain.Dependencies.String()},
		{Path: "projects/rust-app/target", Kind: domain.BuildOutput.String()},
		{Path: npmCachePath(), Kind: domain.Cache.String()},
	},
	Traps: []trap{
		{Path: "projects/tracked-app/node_modules", Reason: "Git tracks its files, so the Git rule keeps it."},
		{Path: "outside/stray-app/node_modules", Reason: "It is outside every Root, so the Root rule keeps it."},
		{Path: "projects/linked-app/through/node_modules", Reason: "It is reached through a Link, so the Root rule keeps it."},
		{Path: "projects/plain/target", Reason: "No Cargo.toml sits beside it, so it is not a Candidate."},
		{Path: "projects/shifted-app/node_modules", Reason: "It is changed between the Plan and the Clean, so the Fingerprint rule keeps it."},
	},
}

func manifest(name string) file {
	return file(fmt.Sprintf("{\n  \"name\": %q,\n  \"version\": \"1.0.0\"\n}\n", name))
}

func module(name string) file {
	return file(fmt.Sprintf("module.exports = %q;\n", name))
}

// license is about 64 KiB, so counting its hard-linked pair twice shows in du.
func license() file {
	var b strings.Builder
	for i := 1; b.Len() < 64<<10; i++ {
		fmt.Fprintf(&b, "Line %d of the license that fake-alpha and fake-beta share.\n", i)
	}
	return file(b.String())
}

func build(dir string) error {
	if err := claim(dir); err != nil {
		return err
	}
	if err := fill(dir); err != nil {
		return fmt.Errorf("%s holds a partial Fixture: %w", dir, err)
	}
	return nil
}

func claim(dir string) error {
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return os.MkdirAll(dir, 0o755)
	case err != nil:
		return err
	case !info.IsDir():
		return fmt.Errorf("%s is a file, so nothing was built: the Fixture goes only into an empty or missing directory", dir)
	}
	inside, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(inside) > 0 {
		return fmt.Errorf("%s is not empty, so nothing was built: the Fixture goes only into an empty or missing directory", dir)
	}
	return nil
}

func fill(dir string) error {
	for _, group := range [][]entry{committed, ignored} {
		for _, e := range group {
			if err := writeEntry(dir, e); err != nil {
				return err
			}
		}
	}
	for _, r := range repositories {
		if err := commitAll(filepath.Join(dir, filepath.FromSlash(r))); err != nil {
			return err
		}
	}
	for _, e := range untracked {
		if err := writeEntry(dir, e); err != nil {
			return err
		}
	}
	for _, e := range npmCache() {
		if err := writeEntry(dir, e); err != nil {
			return err
		}
	}
	for _, r := range repositories {
		if err := tracksAll(dir, r); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(key, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "answer-key.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	return stampAges(dir)
}

// stampAges sets the modification times the Stale and recently used Projects
// are recognised by. The Stale Project's own files are old and their access
// time is now, and the files inside its Candidate are newer than they are,
// so neither an access time nor a file inside the Candidate can be what a
// Scan reports as Last used.
func stampAges(dir string) error {
	now := time.Now()
	hour := now.Add(-time.Hour)
	if err := stampProject(dir, "projects/stale-app", now.Add(-60*24*time.Hour), hour, now); err != nil {
		return err
	}
	if err := stampProject(dir, "projects/recent-app", now.Add(-24*time.Hour), hour, now.Add(-24*time.Hour)); err != nil {
		return err
	}
	old := now.Add(-60 * 24 * time.Hour)
	cache := filepath.Join(dir, filepath.FromSlash(npmCachePath()))
	return filepath.WalkDir(cache, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		return os.Chtimes(p, old, old)
	})
}

func stampProject(dir, project string, own, inside, accessed time.Time) error {
	root := filepath.Join(dir, filepath.FromSlash(project))
	candidate := filepath.Join(root, "node_modules")
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		modified, access := own, accessed
		if p == candidate || strings.HasPrefix(p, candidate+string(filepath.Separator)) {
			modified, access = inside, inside
		}
		return os.Chtimes(p, access, modified)
	})
}

func writeEntry(dir string, e entry) error {
	target := filepath.Join(dir, filepath.FromSlash(e.path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return e.content.create(dir, target)
}

func commitAll(repo string) error {
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"commit", "-q", "-m", "Track node_modules"},
		// read-tree drops the inodes and timestamps commit left in the index, so two builds match byte for byte.
		{"read-tree", "HEAD"},
	} {
		if _, err := git(repo, args...); err != nil {
			return err
		}
	}
	return nil
}

// tracksAll fails the build when the Trap's files are not tracked, or when
// an ignored or untracked Candidate is tracked, because either mistake
// would make a Scan show the wrong paths.
func tracksAll(dir, r string) error {
	repo := filepath.Join(dir, filepath.FromSlash(r))
	out, err := git(repo, "ls-files", "-z")
	if err != nil {
		return err
	}
	tracked := strings.Split(string(out), "\x00")
	var missing, present []string
	for _, e := range committed {
		if p, ok := strings.CutPrefix(e.path, r+"/"); ok && !slices.Contains(tracked, p) {
			missing = append(missing, p)
		}
	}
	for _, e := range append(slices.Clone(ignored), untracked...) {
		if p, ok := strings.CutPrefix(e.path, r+"/"); ok && slices.Contains(tracked, p) {
			present = append(present, p)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("git does not track %s in %s, so the Trap would not hold", strings.Join(missing, ", "), r)
	}
	if len(present) > 0 {
		return fmt.Errorf("git tracks %s in %s, so that Candidate would be a Trap", strings.Join(present, ", "), r)
	}
	for _, e := range ignored {
		if p, ok := strings.CutPrefix(e.path, r+"/"); ok {
			if err := wantIgnored(repo, p, true); err != nil {
				return err
			}
		}
	}
	for _, e := range untracked {
		if p, ok := strings.CutPrefix(e.path, r+"/"); ok {
			if err := wantIgnored(repo, p, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func wantIgnored(repo, rel string, ignored bool) error {
	code, err := gitCode(repo, "check-ignore", "-q", "--", rel)
	if err != nil {
		return err
	}
	switch {
	case ignored && code != 0:
		return fmt.Errorf("git does not ignore %s, so the ignored Candidate would be only untracked", rel)
	case !ignored && code == 0:
		return fmt.Errorf("git ignores %s, so the untracked Candidate would be ignored", rel)
	case code != 0 && code != 1:
		return fmt.Errorf("git check-ignore %s: exit %d", rel, code)
	}
	return nil
}

func git(dir string, args ...string) ([]byte, error) {
	out, code, err := runGit(dir, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("git %s in %s: exit %d: %s", strings.Join(args, " "), dir, code, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func gitCode(dir string, args ...string) (int, error) {
	_, code, err := runGit(dir, args...)
	return code, err
}

func runGit(dir string, args ...string) ([]byte, int, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return stderr.Bytes(), exit.ExitCode(), nil
	}
	return nil, 0, fmt.Errorf("git %s in %s: %w", strings.Join(args, " "), dir, err)
}

// gitEnv shuts out every git setting from outside the repository, such as
// commit signing, line endings or a default branch, and the per-user ignore
// and attributes files git reads even with no config, so none of them changes
// the Fixture.
func gitEnv() []string {
	env := []string{
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=core.excludesFile",
		"GIT_CONFIG_VALUE_0=" + os.DevNull,
		"GIT_CONFIG_KEY_1=core.attributesFile",
		"GIT_CONFIG_VALUE_1=" + os.DevNull,
		"GIT_AUTHOR_NAME=Fixture",
		"GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=Fixture",
		"GIT_COMMITTER_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") {
			env = append(env, v)
		}
	}
	return env
}
