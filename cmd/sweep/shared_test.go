package main_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTheNpmCacheIsASharedCache(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.home = filepath.Join(dir, "home")
	b.config = t.TempDir()
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	cache := cacheCandidate(t, dir, key)
	root := at(key.Root)
	before := listing(t, dir)
	t.Cleanup(func() {
		for _, d := range differences(before, listing(t, dir)) {
			t.Errorf("the Fixture changed during a Scan: %s", d)
		}
	})

	got := b.run(t, dir, "scan", "--json", root)
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
	}
	out := decode(t, got)
	if len(out.Added) != 1 || !sameDir(t, out.Added[0], cache) {
		t.Fatalf("added = %v, want %s", out.Added, cache)
	}
	var found bool
	for _, c := range out.Candidates {
		if !sameDir(t, c.Path, cache) {
			continue
		}
		found = true
		if c.Kind != "Cache" || c.Ecosystem != "Node" || c.Project != "" || !strings.HasSuffix(c.Reason, ".") {
			t.Errorf("candidate = %+v, want a Node Cache with no Project and a Reason", c)
		}
		onDisk := du(t, cache)
		if c.Size != onDisk {
			t.Errorf("Size = %d, and du -s gives %d", c.Size, onDisk)
		}
		if !c.Stale {
			t.Errorf("Stale = false, and the cache's files are older than 30 days")
		}
	}
	if !found {
		t.Fatalf("the npm cache is not listed in\n%s", got.stdout)
	}

	table := b.run(t, dir, "scan", root)
	if table.status != 0 || table.stderr != "" {
		t.Fatalf("table exit %d, stderr %q", table.status, table.stderr)
	}
	if !strings.Contains(table.stdout, "\nShared caches\n") || !strings.Contains(table.stdout, cache+"  added by sweep") {
		t.Errorf("the table does not list the cache in its own group, marked added by sweep\n%s", table.stdout)
	}

	off := b.run(t, dir, "scan", "--json", "--no-shared-caches", root)
	if off.status != 0 || off.stderr != "" {
		t.Fatalf("--no-shared-caches exit %d, stderr %q", off.status, off.stderr)
	}
	quiet := decode(t, off)
	if len(quiet.Added) != 0 {
		t.Errorf("added = %v, want none with Shared caches off", quiet.Added)
	}
	for _, c := range quiet.Candidates {
		if sameDir(t, c.Path, cache) {
			t.Errorf("--no-shared-caches listed %s", cache)
		}
	}
	if len(quiet.Roots) != 1 || !sameDir(t, quiet.Roots[0], root) {
		t.Errorf("roots = %v, want only %s", quiet.Roots, root)
	}

	elsewhere := b.run(t, dir, "scan", "--json", t.TempDir())
	if elsewhere.status != 0 {
		t.Fatalf("exit status %d, stderr %q", elsewhere.status, elsewhere.stderr)
	}
	away := decode(t, elsewhere)
	if len(away.Candidates) != 1 || !sameDir(t, away.Candidates[0].Path, cache) {
		t.Errorf("candidates = %+v, want the npm cache even when the Root holds nothing", away.Candidates)
	}
}

func TestTheSharedCachesSettingTurnsThemOff(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.home = filepath.Join(dir, "home")
	b.config = t.TempDir()
	cfg := b.config
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		cfg = b.home
	}
	writeConfig(t, cfg, "shared_caches = false\n")
	cache := cacheCandidate(t, dir, key)

	got := b.run(t, dir, "scan", "--json", filepath.Join(dir, filepath.FromSlash(key.Root)))
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
	}
	out := decode(t, got)
	for _, c := range out.Candidates {
		if sameDir(t, c.Path, cache) {
			t.Errorf("shared_caches = false still listed %s", cache)
		}
	}
	if len(out.Added) != 0 {
		t.Errorf("added = %v, want none", out.Added)
	}
}

func TestASharedCacheIsNotInTheDefaultPick(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.home = filepath.Join(dir, "home")
	b.config = t.TempDir()
	cache := cacheCandidate(t, dir, key)
	root := filepath.Join(dir, filepath.FromSlash(key.Root))
	planPath := filepath.Join(t.TempDir(), "plan.json")
	before := listing(t, dir)

	got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, root)
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit status %d, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	pickedAt := strings.Index(got.stdout, "picked: Stale\n")
	chooseAt := strings.Index(got.stdout, "not picked: choose it yourself\n")
	if pickedAt < 0 || chooseAt < pickedAt {
		t.Fatalf("stdout is\n%s\nwant picked: Stale and then not picked: choose it yourself", got.stdout)
	}
	if strings.Contains(got.stdout[pickedAt:chooseAt], cache) || !strings.Contains(got.stdout[chooseAt:], cache) {
		t.Errorf("the cache is not under not picked: choose it yourself\n%s", got.stdout)
	}
	out := decodePlan(t, planPath)
	if planHolds(t, out, cache) {
		t.Errorf("the Plan holds %s, and a Shared cache is never picked by default", cache)
	}
	var added bool
	for _, r := range out.Roots {
		if sameDir(t, r, cache) {
			added = true
		}
	}
	if !added {
		t.Errorf("Plan roots = %v, want the Shared cache written in the Plan", out.Roots)
	}
	stale := filepath.Join(dir, "projects", "stale-app", "node_modules")
	if !planHolds(t, out, stale) {
		t.Errorf("the Plan does not hold the Stale Candidate %s", stale)
	}

	picked := filepath.Join(t.TempDir(), "picked.json")
	named := b.run(t, dir, "clean", "--dry-run", "--plan", picked, "--pick-caches", "Node", root)
	if named.status != 0 || named.stderr != "" {
		t.Fatalf("--pick-caches exit %d, stderr %q\n%s", named.status, named.stderr, named.stdout)
	}
	if !planHolds(t, decodePlan(t, picked), cache) {
		t.Errorf("--pick-caches Node left the npm cache out of the Plan")
	}
	if !strings.Contains(named.stdout, "picked: named with --pick-caches") {
		t.Errorf("stdout does not say the Ecosystem was named\n%s", named.stdout)
	}

	nopePath := filepath.Join(t.TempDir(), "nope.json")
	missing := b.run(t, dir, "clean", "--dry-run", "--plan", nopePath, "--pick-caches", "Nope", root)
	if missing.status != 2 || missing.stdout != "" || !strings.Contains(missing.stderr, "Nope") {
		t.Errorf("--pick-caches Nope: exit %d, stdout %q, stderr %q", missing.status, missing.stdout, missing.stderr)
	}
	if _, err := os.Lstat(nopePath); !os.IsNotExist(err) {
		t.Errorf("a refused Pick wrote a Plan: %v", err)
	}

	for _, d := range differences(before, listing(t, dir)) {
		t.Errorf("the Fixture changed during a dry run: %s", d)
	}
}

func TestALinkInAnAncestorStillListsTheNpmCache(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.home = filepath.Join(dir, "home")
	b.config = t.TempDir()
	cache := cacheCandidate(t, dir, key)
	before := listing(t, dir)

	direct := b.run(t, dir, "scan", "--json", b.home)
	if direct.status != 0 || direct.stderr != "" {
		t.Fatalf("direct exit %d, stderr %q", direct.status, direct.stderr)
	}
	if n := listedCache(t, decode(t, direct), cache); n != 1 {
		t.Fatalf("scanning the home lists the cache %d times, want once", n)
	}

	via := filepath.Join(t.TempDir(), "via")
	linkTo(t, dir, via)
	through := b.run(t, dir, "scan", "--json", filepath.Join(via, "home"))
	if through.status != 0 || through.stderr != "" {
		t.Fatalf("through a Link: exit %d, stderr %q\n%s", through.status, through.stderr, through.stdout)
	}
	if n := listedCache(t, decode(t, through), cache); n != 1 {
		t.Fatalf("scanning the home through a Link lists the cache %d times, want once\n%s", n, through.stdout)
	}

	for _, d := range differences(before, listing(t, dir)) {
		t.Errorf("the Fixture changed: %s", d)
	}
}

func TestAnIgnoredUnreadableCacheParentDoesNotFailTheScan(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("only an account that is not root can be refused a directory by its mode")
	}
	b := build(t)
	dir, key := b.buildFixture(t)
	b.home = filepath.Join(dir, "home")
	b.config = t.TempDir()
	parent := filepath.Join(b.home, ".npm")
	if err := os.Chmod(parent, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	root := filepath.Join(dir, filepath.FromSlash(key.Root))

	open := b.run(t, dir, "scan", "--json", root)
	if open.status != 1 {
		t.Fatalf("exit status %d, want 1 while the cache parent cannot be read", open.status)
	}
	if !strings.Contains(open.stdout, parent) {
		t.Fatalf("unreadable does not name %s\n%s", parent, open.stdout)
	}

	cfg := b.config
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		cfg = b.home
	}
	writeConfig(t, cfg, "ignore = ["+tomlPath(t, parent)+"]\n")
	got := b.run(t, dir, "scan", "--json", root)
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit status %d, stderr %q, want 0\n%s", got.status, got.stderr, got.stdout)
	}
	out := decode(t, got)
	for _, u := range out.Unreadable {
		if strings.Contains(u.Path, parent) {
			t.Errorf("reported %s, which is ignored", u.Path)
		}
	}
	for _, c := range out.Candidates {
		if strings.Contains(c.Path, "_cacache") {
			t.Errorf("listed %s, and its parent is ignored", c.Path)
		}
	}
}

func TestAScanRunsWhenTheHomeDirectoryIsUnknown(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		t.Skip("the configuration directory and the home directory are both HOME")
	}
	b := build(t)
	top := t.TempDir()
	candidate := filepath.Join(top, "app", "node_modules", "a.js")
	if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, "app", "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configHome := t.TempDir()
	cmd := exec.Command(b.sweep, "scan", "--json", top)
	env := without(os.Environ(), "HOME", "USERPROFILE", "UserProfile", "HOMEDRIVE", "HomeDrive", "HOMEPATH", "HomePath")
	env = append(env, "XDG_CONFIG_HOME="+configHome, "XDG_CACHE_HOME="+t.TempDir(), "XDG_DATA_HOME="+t.TempDir())
	if runtime.GOOS == "windows" {
		env = windowsConfigEnv(env, configHome)
		env = windowsCacheEnv(env, t.TempDir())
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	status := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		status = exit.ExitCode()
	}
	if status != 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d, stderr %q, want the Scan of the given Root", status, stderr.String())
	}
	out := decode(t, ran{stdout: stdout.String()})
	if len(out.Added) != 0 {
		t.Errorf("added = %v, want none when the home directory is unknown", out.Added)
	}
	if len(out.Candidates) != 1 || !sameDir(t, out.Candidates[0].Path, filepath.Join(top, "app", "node_modules")) {
		t.Errorf("candidates = %+v, want app/node_modules", out.Candidates)
	}
}

func listedCache(t *testing.T, out scanJSON, cache string) int {
	t.Helper()
	n := 0
	for _, c := range out.Candidates {
		if sameDir(t, c.Path, cache) {
			n++
		}
	}
	return n
}
