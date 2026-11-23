// Package git asks the git program whether a path holds a file it tracks.
// It only reads. It never runs a git command that writes, and the index of
// a repository it asks about stays byte for byte as it was.
package git

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
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
func (l Local) State(path string) safety.State {
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
