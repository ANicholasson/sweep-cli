package main_test

import (
	"os"
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
