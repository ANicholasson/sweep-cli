package main_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type binaries struct {
	sweep, fixture string
	config         string
}

func build(t *testing.T) binaries {
	t.Helper()
	dir := t.TempDir()
	exe := func(name string) string {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return filepath.Join(dir, name)
	}
	b := binaries{sweep: exe("sweep"), fixture: exe("fixture")}
	command(t, "go", "build", "-o", b.sweep, ".")
	command(t, "go", "build", "-o", b.fixture, "../../tools/fixture")
	return b
}

type answerKey struct {
	Version    int
	Root       string
	Candidates []struct{ Path, Kind string }
	Traps      []struct{ Path, Reason string }
}

func (b binaries) buildFixture(t *testing.T) (string, answerKey) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "fixture")
	command(t, b.fixture, dir)
	data, err := os.ReadFile(filepath.Join(dir, "answer-key.json"))
	if err != nil {
		t.Fatal(err)
	}
	var key answerKey
	if err := json.Unmarshal(data, &key); err != nil {
		t.Fatal(err)
	}
	if key.Version != 1 {
		t.Fatalf("the Answer key is version %d, and these tests read version 1", key.Version)
	}
	return dir, key
}

type ran struct {
	stdout, stderr string
	status         int
}

func (b binaries) run(t *testing.T, dir string, args ...string) ran {
	t.Helper()
	cmd := exec.Command(b.sweep, args...)
	cmd.Dir = dir
	home := b.config
	if home == "" {
		home = t.TempDir()
	}
	cmd.Env = pointConfiguration(os.Environ(), home)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return ran{stdout.String(), stderr.String(), cmd.ProcessState.ExitCode()}
}

// du asks for 512-byte blocks, which GNU du and BSD du both give exactly,
// where -k would round each total up to 1024.
func du(t *testing.T, path string) int64 {
	t.Helper()
	cmd := exec.Command("du", "-s", path)
	cmd.Env = append(without(os.Environ(), "DU_BLOCK_SIZE", "BLOCK_SIZE", "BLOCKSIZE"), "POSIXLY_CORRECT=1", "BLOCKSIZE=512")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("du -s %s, which every Size is checked against, so the gate needs du on the PATH: %v", path, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		t.Fatalf("du -s %s printed nothing", path)
	}
	blocks, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		t.Fatalf("du -s %s printed %q", path, out)
	}
	return blocks * 512
}

// settled gives du for a path once two readings around measure agree, so a
// file system still placing a new file's blocks cannot make a right Size
// look wrong, or a wrong one look right.
func settled(t *testing.T, path string, measure func()) int64 {
	t.Helper()
	for range 5 {
		before := du(t, path)
		measure()
		if du(t, path) == before {
			return before
		}
	}
	t.Fatalf("du -s %s never gave the same answer twice", path)
	return 0
}

func listing(t *testing.T, dir string) map[string]string {
	t.Helper()
	found := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// On Windows d.Info gives a directory's times as its parent's listing
		// last copied them, which can lag behind the directory itself.
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%v %d %s", info.Mode(), info.Size(), info.ModTime().UTC())
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			line += " -> " + target
		case d.Type().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			line += fmt.Sprintf(" %x", sha256.Sum256(data))
		}
		found[p] = line
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func differences(before, after map[string]string) []string {
	var out []string
	for p, was := range before {
		switch is, ok := after[p]; {
		case !ok:
			out = append(out, p+" is gone")
		case is != was:
			out = append(out, p+" was "+was+" and is "+is)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			out = append(out, p+" is new")
		}
	}
	slices.Sort(out)
	return out
}

// sameDir reports whether path a is the directory b, however each is
// spelled, so a short name or a Link in a temporary directory's path cannot
// fail a comparison.
func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	x, err := os.Stat(a)
	if err != nil {
		return false
	}
	y, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return x.IsDir() && os.SameFile(x, y)
}

func command(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func pointConfiguration(env []string, home string) []string {
	switch runtime.GOOS {
	case "windows":
		return append(without(env, "AppData"), "AppData="+home)
	case "darwin", "ios":
		return append(without(env, "HOME"), "HOME="+home)
	default:
		return append(without(env, "XDG_CONFIG_HOME"), "XDG_CONFIG_HOME="+home)
	}
}

func configurationFile(home string) string {
	dir := home
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		dir = filepath.Join(home, "Library", "Application Support")
	}
	return filepath.Join(dir, "sweep", "config.toml")
}

func without(env []string, names ...string) []string {
	var kept []string
	for _, v := range env {
		name, _, _ := strings.Cut(v, "=")
		if !slices.Contains(names, name) {
			kept = append(kept, v)
		}
	}
	return kept
}
