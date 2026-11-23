// Package git asks the git program whether a path holds a file it tracks.
// It only reads. It never runs a git command that writes, and the index of
// a repository it asks about stays byte for byte as it was.
package git

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ANicholasson/sweep-cli/internal/domain/safety"
)

// Local is the git program on this machine.
type Local struct {
	// Bin is the program to run. Empty runs git from PATH.
	Bin string
}

var _ safety.Git = Local{}

// State runs git ls-files in path. An empty listing inside a work tree is
// Untracked, a listing is Tracked, and "not a git repository" is Outside.
// Anything else, including a missing program, is Unknown.
//
// A repository nested inside path is a separate work tree. ls-files in path
// does not list its tracked files, so State asks each nested repository
// before it reports Outside or Untracked. A failure there is Unknown.
func (l Local) State(path string) safety.State {
	state := l.ask(path)
	switch state {
	case safety.Tracked, safety.Unknown:
		return state
	}
	nested, err := l.nested(path)
	if err != nil || nested == safety.Unknown {
		return safety.Unknown
	}
	if nested == safety.Tracked {
		return safety.Tracked
	}
	return state
}

func (l Local) ask(path string) safety.State {
	bin := l.Bin
	if bin == "" {
		bin = "git"
	}
	cmd := exec.Command(bin,
		"-c", "core.fsmonitor=",
		"-c", "core.untrackedCache=false",
		"-C", path,
		"ls-files", "-z")
	cmd.Env = isolatedEnv()
	out, err := cmd.Output()
	if err == nil {
		if len(bytes.Trim(out, "\x00")) == 0 {
			return safety.Untracked
		}
		return safety.Tracked
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 128 && bytes.Contains(exit.Stderr, []byte("not a git repository")) {
		return safety.Outside
	}
	return safety.Unknown
}

// nested reports Tracked when a repository inside path, and not path
// itself, holds a file git tracks. It reports Unknown when that question
// fails. An empty State means no nested repository tracks a file.
func (l Local) nested(root string) (safety.State, error) {
	var state safety.State
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return fs.SkipDir
		}
		if p == root {
			return nil
		}
		_, statErr := os.Lstat(filepath.Join(p, ".git"))
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		switch l.ask(p) {
		case safety.Tracked:
			state = safety.Tracked
			return errStop
		case safety.Untracked:
			return nil
		default:
			state = safety.Unknown
			return errStop
		}
	})
	if errors.Is(err, errStop) {
		return state, nil
	}
	if err != nil {
		return safety.Unknown, err
	}
	return state, nil
}

var errStop = errors.New("git: stop walk")

// isolatedEnv drops every GIT_ variable and the caller's locale, then sets
// the ones this read needs. A GIT_DIR from the environment would answer
// about some other repository, and a config that turns on a file-system
// monitor can make a read rewrite the index.
func isolatedEnv() []string {
	overrides := []string{
		"LC_ALL=C",
		"LANGUAGE=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_OPTIONAL_LOCKS=0",
	}
	skip := map[string]bool{}
	for _, v := range overrides {
		key, _, _ := strings.Cut(v, "=")
		skip[key] = true
	}
	env := append([]string{}, overrides...)
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if skip[key] || strings.HasPrefix(key, "GIT_") {
			continue
		}
		env = append(env, v)
	}
	return env
}
