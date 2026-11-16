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
	"slices"
	"strings"

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

var fixture = []entry{
	{"projects/web-app/package.json", manifest("web-app")},
	{"projects/web-app/src/index.js", file(`console.log(require("fake-alpha"), require("fake-beta"));` + "\n")},
	{"projects/web-app/node_modules/fake-alpha/package.json", manifest("fake-alpha")},
	{"projects/web-app/node_modules/fake-alpha/index.js", module("fake-alpha")},
	{"projects/web-app/node_modules/fake-alpha/LICENSE", license()},
	{"projects/web-app/node_modules/fake-beta/package.json", manifest("fake-beta")},
	{"projects/web-app/node_modules/fake-beta/index.js", module("fake-beta")},
	{"projects/web-app/node_modules/fake-beta/LICENSE", hardLink("projects/web-app/node_modules/fake-alpha/LICENSE")},

	{"projects/tracked-app/package.json", manifest("tracked-app")},
	{"projects/tracked-app/node_modules/fake-gamma/package.json", manifest("fake-gamma")},
	{"projects/tracked-app/node_modules/fake-gamma/index.js", module("fake-gamma")},

	{"outside/stray-app/package.json", manifest("stray-app")},
	{"outside/stray-app/node_modules/fake-delta/package.json", manifest("fake-delta")},
	{"outside/stray-app/node_modules/fake-delta/index.js", module("fake-delta")},
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
	},
	Traps: []trap{
		{Path: "projects/tracked-app/node_modules", Reason: "Git tracks its files, so the Git rule keeps it."},
		{Path: "outside/stray-app/node_modules", Reason: "It is outside every Root, so the Root rule keeps it."},
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
	for _, e := range fixture {
		target := filepath.Join(dir, filepath.FromSlash(e.path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := e.content.create(dir, target); err != nil {
			return err
		}
	}
	for _, r := range repositories {
		if err := commitAll(filepath.Join(dir, filepath.FromSlash(r))); err != nil {
			return err
		}
		if err := tracksAll(dir, r); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(key, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "answer-key.json"), append(data, '\n'), 0o644)
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

// tracksAll fails the build when git skips a file of repository r, because a
// skipped file would leave the Trap untracked with no other sign.
func tracksAll(dir, r string) error {
	out, err := git(filepath.Join(dir, filepath.FromSlash(r)), "ls-files", "-z")
	if err != nil {
		return err
	}
	tracked := strings.Split(string(out), "\x00")
	var missing []string
	for _, e := range fixture {
		if p, ok := strings.CutPrefix(e.path, r+"/"); ok && !slices.Contains(tracked, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("git does not track %s in %s, so the Trap would not hold", strings.Join(missing, ", "), r)
	}
	return nil
}

func git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	if said := strings.TrimSpace(stderr.String()); said != "" {
		err = fmt.Errorf("%w: %s", err, said)
	}
	return nil, fmt.Errorf("git %s in %s: %w", strings.Join(args, " "), dir, err)
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
