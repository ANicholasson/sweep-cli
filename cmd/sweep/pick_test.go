package main_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestPickByKindEcosystemAndAge(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	// The npm cache is a Shared cache. These flags do not pick one, and this
	// dry run's home directory is not the Fixture's home, so it is not listed.
	var projects []struct{ Path, Kind string }
	for _, c := range key.Candidates {
		if strings.HasPrefix(filepath.ToSlash(c.Path), "home/") {
			continue
		}
		projects = append(projects, c)
	}
	key.Candidates = projects
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	root := at(key.Root)
	before := listing(t, dir)
	t.Cleanup(func() {
		for _, d := range differences(before, listing(t, dir)) {
			t.Errorf("the Fixture changed during a dry run: %s", d)
		}
	})

	t.Run("one Kind", func(t *testing.T) {
		got, out := dry(t, b, dir, "--kind", "Dependencies", root)
		want := pickedBy(t, key, at, []string{"Dependencies"}, "", 30, false)
		assertPlan(t, out, key, at, want)
		if strings.Contains(got.stdout, "Docker") {
			t.Errorf("stdout names Docker data\n%s", got.stdout)
		}
		for _, c := range key.Candidates {
			path := at(c.Path)
			switch {
			case samePath(t, want, path):
				assertUnder(t, got.stdout, "picked: Stale", path)
			case c.Kind != "Dependencies":
				assertUnder(t, got.stdout, "not picked: not named with --kind", path)
			default:
				assertUnder(t, got.stdout, "not picked: used recently", path)
			}
		}
	})

	t.Run("two Kinds and --all", func(t *testing.T) {
		_, out := dry(t, b, dir, "--kind", "Dependencies", "--kind", "Build output", "--all", root)
		want := pickedBy(t, key, at, []string{"Dependencies", "Build output"}, "", 30, true)
		assertPlan(t, out, key, at, want)
	})

	t.Run("one Ecosystem", func(t *testing.T) {
		got, out := dry(t, b, dir, "--ecosystem", "Cargo", "--all", root)
		want := pickedBy(t, key, at, nil, "Cargo.toml", 30, true)
		assertPlan(t, out, key, at, want)
		for _, c := range key.Candidates {
			path := at(c.Path)
			if hasManifest(path, "package.json") {
				assertUnder(t, got.stdout, "not picked: not named with --ecosystem", path)
			}
		}
		for _, path := range want {
			assertUnder(t, got.stdout, "picked: --all", path)
		}
	})

	t.Run("Kind and Ecosystem together", func(t *testing.T) {
		got, out := dry(t, b, dir, "--kind", "Build output", "--ecosystem", "Node", "--all", root)
		want := pickedBy(t, key, at, []string{"Build output"}, "package.json", 30, true)
		assertPlan(t, out, key, at, want)
		if len(want) != 0 {
			t.Fatalf("want no Node Build output in this Fixture, got %v", want)
		}
		for _, c := range key.Candidates {
			path := at(c.Path)
			if c.Kind != "Build output" {
				assertUnder(t, got.stdout, "not picked: not named with --kind", path)
				continue
			}
			assertUnder(t, got.stdout, "not picked: not named with --ecosystem", path)
		}
	})

	t.Run("the age for one run", func(t *testing.T) {
		for _, days := range []int{90, 0} {
			got, out := dry(t, b, dir, "--stale-days", strconv.Itoa(days), root)
			want := pickedBy(t, key, at, nil, "", days, false)
			assertPlan(t, out, key, at, want)
			for _, c := range key.Candidates {
				path := at(c.Path)
				if samePath(t, want, path) {
					assertUnder(t, got.stdout, "picked: older than --stale-days "+strconv.Itoa(days), path)
				} else {
					assertUnder(t, got.stdout, "not picked: newer than --stale-days "+strconv.Itoa(days), path)
				}
			}
		}
	})

	t.Run("the flag replaces the configuration threshold", func(t *testing.T) {
		saved := b.config
		b.config = t.TempDir()
		t.Cleanup(func() { b.config = saved })
		writeConfig(t, b.config, "stale_days = 90\n")
		_, out := dry(t, b, dir, "--stale-days", "0", root)
		want := pickedBy(t, key, at, nil, "", 0, false)
		assertPlan(t, out, key, at, want)
		if !samePath(t, want, at("projects/stale-app/node_modules")) {
			t.Fatal("the 60-day Project is not Stale at 0 days, so this does not show the flag winning")
		}
	})

	t.Run("every Candidate whatever its age", func(t *testing.T) {
		got, out := dry(t, b, dir, "--all", root)
		want := pickedBy(t, key, at, nil, "", 30, true)
		assertPlan(t, out, key, at, want)
		if strings.Contains(got.stdout, "picked: Stale") || strings.Contains(got.stdout, "Docker") {
			t.Errorf("stdout is\n%s\nwant --all and no Docker data", got.stdout)
		}
		for _, path := range want {
			assertUnder(t, got.stdout, "picked: --all", path)
		}
	})

	t.Run("--pick adds a Candidate a Kind left out", func(t *testing.T) {
		extra := at("projects/recent-app/node_modules")
		got, out := dry(t, b, dir, "--kind", "Build output", "--pick", extra, root)
		want := pickedBy(t, key, at, []string{"Build output"}, "", 30, false)
		want = append(want, extra)
		assertPlan(t, out, key, at, want)
		assertUnder(t, got.stdout, "picked: named with --pick", extra)
	})

	t.Run("an unknown Kind writes no Plan", func(t *testing.T) {
		planPath := filepath.Join(t.TempDir(), "plan.json")
		got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, "--kind", "dependencies", root)
		if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, "unknown Kind") {
			t.Fatalf("exit status %d, stdout %q, stderr %q", got.status, got.stdout, got.stderr)
		}
		if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
			t.Errorf("wrote a Plan for an unknown Kind: %v", err)
		}
	})

	t.Run("an unknown Ecosystem writes no Plan", func(t *testing.T) {
		planPath := filepath.Join(t.TempDir(), "plan.json")
		got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, "--ecosystem", "Python", root)
		if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, "unknown Ecosystem") {
			t.Fatalf("exit status %d, stdout %q, stderr %q", got.status, got.stdout, got.stderr)
		}
		if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
			t.Errorf("wrote a Plan for an unknown Ecosystem: %v", err)
		}
	})

	t.Run("a bad age writes no Plan", func(t *testing.T) {
		planPath := filepath.Join(t.TempDir(), "plan.json")
		got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, "--stale-days", "-1", root)
		if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, "not a whole number of days") {
			t.Fatalf("exit status %d, stdout %q, stderr %q", got.status, got.stdout, got.stderr)
		}
		if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
			t.Errorf("wrote a Plan for a bad age: %v", err)
		}
	})

	t.Run("the flags do nothing without a dry run", func(t *testing.T) {
		planPath := filepath.Join(t.TempDir(), "plan.json")
		made := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, root)
		if made.status != 0 || made.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", made.status, made.stderr)
		}
		target := at("projects/stale-app/node_modules")
		got := b.run(t, dir, "clean", "--plan", planPath, "--all")
		if got.status != 2 || !strings.Contains(got.stderr, "--all") {
			t.Fatalf("exit status %d, stderr %q, want a usage error", got.status, got.stderr)
		}
		if _, err := os.Lstat(target); err != nil {
			t.Errorf("the Candidate is gone after a refused Clean: %v", err)
		}
	})

	t.Run("the usage names the flags and not Docker data", func(t *testing.T) {
		got := b.run(t, dir, "clean", "-h")
		if got.status != 2 || got.stdout != "" {
			t.Fatalf("exit status %d, stdout %q", got.status, got.stdout)
		}
		for _, flag := range []string{"--kind", "--ecosystem", "--stale-days", "--all"} {
			if !strings.Contains(got.stderr, flag) {
				t.Errorf("stderr does not name %s\n%s", flag, got.stderr)
			}
		}
		if strings.Contains(strings.ToLower(got.stderr), "docker") {
			t.Errorf("stderr names Docker data\n%s", got.stderr)
		}
	})
}

func dry(t *testing.T, b binaries, dir string, args ...string) (ran, planJSON) {
	t.Helper()
	planPath := filepath.Join(t.TempDir(), "plan.json")
	full := append([]string{"clean", "--dry-run", "--plan", planPath}, args...)
	got := b.run(t, dir, full...)
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit status %d, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	return got, decodePlan(t, planPath)
}

func assertPlan(t *testing.T, out planJSON, key answerKey, at func(string) string, want []string) {
	t.Helper()
	if paths := planPaths(t, out); !sameSet(t, paths, want) {
		t.Errorf("Plan paths = %v, want %v", paths, want)
	}
	for _, c := range out.Candidates {
		onDisk := du(t, c.Path)
		if c.Fingerprint.Size != onDisk {
			t.Errorf("%s: Fingerprint Size = %d, and du -s gives %d", c.Path, c.Fingerprint.Size, onDisk)
		}
	}
	for _, trap := range key.Traps {
		if candidateToo(key, trap.Path) {
			continue
		}
		if planHolds(t, out, at(trap.Path)) {
			t.Errorf("the Trap %s is in the Plan", trap.Path)
		}
	}
}

func pickedBy(t *testing.T, key answerKey, at func(string) string, kinds []string, manifest string, days int, anyAge bool) []string {
	t.Helper()
	var paths []string
	for _, c := range key.Candidates {
		path := at(c.Path)
		if len(kinds) > 0 && !slices.Contains(kinds, c.Kind) {
			continue
		}
		if manifest != "" && !hasManifest(path, manifest) {
			continue
		}
		if !anyAge && !staleBefore(t, filepath.Dir(path), path, days) {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func hasManifest(candidate, name string) bool {
	_, err := os.Lstat(filepath.Join(filepath.Dir(candidate), name))
	return err == nil
}

func samePath(t *testing.T, paths []string, path string) bool {
	t.Helper()
	for _, p := range paths {
		if sameDir(t, p, path) {
			return true
		}
	}
	return false
}

func assertUnder(t *testing.T, stdout, header, path string) {
	t.Helper()
	at := strings.Index(stdout, header+"\n")
	if at < 0 {
		t.Errorf("stdout is\n%s\nwant a row under %s", stdout, header)
		return
	}
	rest := stdout[at+len(header)+1:]
	end := len(rest)
	for _, mark := range []string{"\npicked:", "\nnot picked:", "\n\n"} {
		if i := strings.Index(rest, mark); i >= 0 && i < end {
			end = i
		}
	}
	if !strings.Contains(rest[:end], path) {
		t.Errorf("%s is not under %s\n%s", path, header, stdout)
	}
}
