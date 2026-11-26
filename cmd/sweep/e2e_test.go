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
	cache          string
	// home is the directory sweep treats as the user's home. Empty uses
	// the configuration directory, which holds no Shared cache, so a run
	// never reads the machine's home.
	home string
	// data is the directory XDG_DATA_HOME points at, so a Clean's trash is
	// there and never in the machine's own trash. Empty uses a fresh
	// temporary directory. omitData leaves XDG_DATA_HOME unset, so Windows
	// uses the Recycle Bin. A test that sets omitData restores what it moves.
	data     string
	omitData bool
	// pwd is the PWD a shell would export for the working directory. Empty
	// leaves the environment's PWD. Set it when that directory is reached
	// through a Link and the Plan should keep that spelling.
	pwd string
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
	configHome, cacheHome := b.userHomes(t)
	dataHome := b.data
	if dataHome == "" && !b.omitData {
		dataHome = t.TempDir()
	}
	if runtime.GOOS == "darwin" && b.data != "" && b.config == "" {
		configHome = dataHome
		cacheHome = dataHome
		ensureDarwinHome(t, dataHome)
	}
	env := pointUserDirs(os.Environ(), configHome, cacheHome, dataHome, b.home)
	if b.omitData {
		env = without(env, "XDG_DATA_HOME")
	}
	if b.pwd != "" {
		env = append(without(env, "PWD"), "PWD="+b.pwd)
	}
	cmd.Env = env
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

// userHomes is the configuration directory and the cache directory for one
// run. An empty field is a fresh temporary directory, so a run never reads
// or writes the real per-user directories. On macOS both come from HOME, so
// a test that sets one gets the other under that same home.
func (b binaries) userHomes(t *testing.T) (configHome, cacheHome string) {
	t.Helper()
	configHome = b.config
	if configHome == "" {
		configHome = t.TempDir()
	}
	cacheHome = b.cache
	if cacheHome == "" {
		cacheHome = t.TempDir()
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		switch {
		case b.cache != "" && b.config == "":
			configHome = cacheHome
		default:
			cacheHome = configHome
		}
	}
	return configHome, cacheHome
}

func pointUserDirs(env []string, configHome, cacheHome, dataHome, home string) []string {
	env = append(without(env, "XDG_DATA_HOME"), "XDG_DATA_HOME="+dataHome)
	if home == "" {
		home = configHome
	}
	switch runtime.GOOS {
	case "windows":
		env = windowsConfigEnv(env, configHome)
		env = windowsCacheEnv(env, cacheHome)
		return append(without(env, "USERPROFILE"), "USERPROFILE="+home)
	case "darwin", "ios":
		return append(without(env, "HOME"), "HOME="+home)
	default:
		env = append(without(env, "XDG_CONFIG_HOME"), "XDG_CONFIG_HOME="+configHome)
		env = append(without(env, "XDG_CACHE_HOME"), "XDG_CACHE_HOME="+cacheHome)
		return append(without(env, "HOME"), "HOME="+home)
	}
}

// windowsCacheEnv points the cache directory at cache. LocalAppData is the
// name Windows sets. The comparison is case-sensitive, so leaving that name
// in place keeps the real local directory.
func windowsCacheEnv(env []string, cache string) []string {
	return append(without(env, "LOCALAPPDATA", "LocalAppData"), "LocalAppData="+cache)
}

// windowsConfigEnv points the configuration directory at home. APPDATA is
// the name Windows sets. The comparison is case-sensitive, so leaving
// that name in place keeps the real roaming directory.
func windowsConfigEnv(env []string, home string) []string {
	return append(without(env, "APPDATA", "AppData"), "AppData="+home)
}

func TestTheRealWindowsRoamingDirectoryIsDropped(t *testing.T) {
	home := filepath.Join(t.TempDir(), "config")
	got := windowsConfigEnv([]string{
		"APPDATA=C:\\Users\\real\\AppData\\Roaming",
		"AppData=C:\\Users\\real\\AppData\\Roaming",
		"PATH=x",
	}, home)
	var n int
	for _, v := range got {
		name, value, _ := strings.Cut(v, "=")
		if name != "APPDATA" && name != "AppData" {
			continue
		}
		n++
		if value != home {
			t.Errorf("%s=%s, want the temporary directory", name, value)
		}
	}
	if n != 1 {
		t.Errorf("%d configuration directories in %v, want one", n, got)
	}
}

func TestTheRealWindowsCacheDirectoryIsDropped(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	got := windowsCacheEnv([]string{
		"LOCALAPPDATA=C:\\AppData\\Local",
		"LocalAppData=C:\\AppData\\Local",
		"PATH=x",
	}, cache)
	var n int
	for _, v := range got {
		name, value, _ := strings.Cut(v, "=")
		if name != "LOCALAPPDATA" && name != "LocalAppData" {
			continue
		}
		n++
		if value != cache {
			t.Errorf("%s=%s, want the temporary directory", name, value)
		}
	}
	if n != 1 {
		t.Errorf("%d cache directories in %v, want one", n, got)
	}
}

func movesToTrash() bool {
	return runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

// ensureDarwinHome creates home when a Clean will use it as HOME. A path
// that already exists is left alone, including a file that blocks the trash
// and a Link the test pointed at a real directory.
func ensureDarwinHome(t *testing.T, home string) {
	t.Helper()
	_, err := os.Lstat(home)
	if err == nil {
		return
	}
	if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
}

func configurationFile(home string) string {
	dir := home
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		dir = filepath.Join(home, "Library", "Application Support")
	}
	return filepath.Join(dir, "sweep", "config.toml")
}

// planFile is the default Plan path when the cache directory is pointed at
// home. home is that cache directory, except on macOS, where it is HOME.
func planFile(home string) string {
	dir := home
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		dir = filepath.Join(home, "Library", "Caches")
	}
	return filepath.Join(dir, "sweep", "plan.json")
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
