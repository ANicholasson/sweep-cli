package main_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDryRunOfTheFixture(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	root := at(key.Root)
	before := listing(t, dir)
	t.Cleanup(func() {
		for _, d := range differences(before, listing(t, dir)) {
			t.Errorf("the Fixture changed during a dry run: %s", d)
		}
	})

	stale, recent := splitByAge(t, key, at)
	if len(stale) == 0 || len(recent) == 0 {
		t.Fatalf("stale = %d, recent = %d, want both, from the files' modification times", len(stale), len(recent))
	}

	t.Run("the Plan holds the Stale Candidates and no Trap", func(t *testing.T) {
		planPath := filepath.Join(t.TempDir(), "plan.json")
		got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, root)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		out := decodePlan(t, planPath)
		if len(out.Roots) != 1 || !sameDir(t, out.Roots[0], root) {
			t.Errorf("roots = %v, want %s", out.Roots, root)
		}
		if paths := planPaths(t, out); !sameSet(t, paths, stale) {
			t.Errorf("Plan paths = %v, want the Stale Candidates %v", paths, stale)
		}
		for _, trap := range key.Traps {
			for _, got := range out.Candidates {
				if sameDir(t, got.Path, at(trap.Path)) {
					t.Errorf("the Trap %s is in the Plan", trap.Path)
				}
			}
			if strings.Contains(got.stdout, at(trap.Path)) {
				t.Errorf("the Trap %s is printed", trap.Path)
			}
		}
		if strings.Contains(got.stdout, "Docker") {
			t.Errorf("stdout mentions Docker, which is never part of a Plan:\n%s", got.stdout)
		}
		assertSections(t, got.stdout, stale, recent)
		var pickedSum, leftSum int64
		for _, path := range stale {
			pickedSum += du(t, path)
		}
		for _, path := range recent {
			leftSum += du(t, path)
		}
		wantSummary := fmt.Sprintf("%s in the Plan, %s\nWritten to %s\nLeft out %s, %s, used recently\nTo add one, pass --pick and its path.\n",
			countPhrase(len(stale)), shown(pickedSum), planPath, countPhrase(len(recent)), shown(leftSum))
		if !strings.Contains(got.stdout, wantSummary) {
			t.Errorf("stdout is\n%s\nwant it to end with\n%s", got.stdout, wantSummary)
		}
		for _, c := range out.Candidates {
			onDisk := du(t, c.Path)
			if c.Fingerprint.Size != onDisk {
				t.Errorf("%s: Fingerprint Size = %d, and du -s gives %d", c.Path, c.Fingerprint.Size, onDisk)
			}
			entries, newest := census(t, c.Path)
			if c.Fingerprint.Entries != entries {
				t.Errorf("%s: entries = %d, and a listing gives %d", c.Path, c.Fingerprint.Entries, entries)
			}
			if !c.Fingerprint.Newest.Equal(newest) {
				t.Errorf("%s: newest = %s, and the listing's newest is %s", c.Path, c.Fingerprint.Newest, newest)
			}
			vol, file := diskID(t, c.Path)
			if c.Fingerprint.Identity.Volume != vol || c.Fingerprint.Identity.File != file {
				t.Errorf("%s: identity = %d:%d, and the file system says %d:%d", c.Path, c.Fingerprint.Identity.Volume, c.Fingerprint.Identity.File, vol, file)
			}
		}
	})

	t.Run("the default file is sweep-plan.json in the working directory", func(t *testing.T) {
		work := t.TempDir()
		got := b.run(t, work, "clean", "--dry-run", root)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		planPath := filepath.Join(work, "sweep-plan.json")
		out := decodePlan(t, planPath)
		if paths := planPaths(t, out); !sameSet(t, paths, stale) {
			t.Errorf("Plan paths = %v, want %v", paths, stale)
		}
		if !strings.Contains(got.stdout, "Written to "+planPath) {
			t.Errorf("stdout is\n%s\nwant the default path", got.stdout)
		}
	})

	t.Run("a Plan inside a Candidate is refused", func(t *testing.T) {
		candidate := at("projects/stale-app/node_modules")
		inside := filepath.Join(candidate, "sweep-plan.json")
		got := b.run(t, dir, "clean", "--dry-run", "--plan", inside, root)
		if got.status != 2 {
			t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want nothing", got.stdout)
		}
		if !strings.Contains(got.stderr, candidate) {
			t.Errorf("stderr = %q, want it to name the Candidate", got.stderr)
		}
		if _, err := os.Lstat(inside); !os.IsNotExist(err) {
			t.Errorf("the Plan was written inside the Candidate: %v", err)
		}
	})

	t.Run("the default Plan is refused when the working directory is a Candidate", func(t *testing.T) {
		candidate := at("projects/stale-app/node_modules")
		got := b.run(t, candidate, "clean", "--dry-run", root)
		if got.status != 2 {
			t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
		}
		if _, err := os.Lstat(filepath.Join(candidate, "sweep-plan.json")); !os.IsNotExist(err) {
			t.Errorf("wrote sweep-plan.json inside the Candidate: %v", err)
		}
	})

	t.Run("--pick adds a Candidate the default left out", func(t *testing.T) {
		planPath := filepath.Join(t.TempDir(), "plan.json")
		extra := recent[0]
		got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, "--pick", extra, root)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		out := decodePlan(t, planPath)
		want := append(append([]string{}, stale...), extra)
		if paths := planPaths(t, out); !sameSet(t, paths, want) {
			t.Errorf("Plan paths = %v, want %v", paths, want)
		}
		if !strings.Contains(got.stdout, "picked: named with --pick") || !strings.Contains(got.stdout, extra) {
			t.Errorf("stdout is\n%s\nwant the named Candidate picked", got.stdout)
		}
		namedAt := strings.Index(got.stdout, "picked: named with --pick")
		leftAt := strings.Index(got.stdout, "not picked: used recently")
		if namedAt < 0 || leftAt < namedAt || !strings.Contains(got.stdout[namedAt:leftAt], extra) {
			t.Errorf("the named Candidate is not under picked: named with --pick\n%s", got.stdout)
		}
	})

	t.Run("a path that is not a Candidate is refused", func(t *testing.T) {
		planPath := filepath.Join(t.TempDir(), "plan.json")
		missing := filepath.Join(dir, "projects", "no-such")
		got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, "--pick", missing, root)
		if got.status != 2 {
			t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
		}
		if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
			t.Errorf("wrote a Plan for a path that is not a Candidate: %v", err)
		}
		if !strings.Contains(got.stderr, "not a Candidate") {
			t.Errorf("stderr = %q, want it to say the path is not a Candidate", got.stderr)
		}
	})

	t.Run("an existing file that is not a Plan is left unchanged", func(t *testing.T) {
		manifest := at("projects/tracked-app/package.json")
		before, err := os.ReadFile(manifest)
		if err != nil {
			t.Fatal(err)
		}
		got := b.run(t, dir, "clean", "--dry-run", "--plan", manifest, root)
		if got.status != 2 {
			t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want nothing", got.stdout)
		}
		if !strings.Contains(got.stderr, "not a Plan") {
			t.Errorf("stderr = %q, want it to say the file is not a Plan", got.stderr)
		}
		after, err := os.ReadFile(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(before) {
			t.Errorf("the file changed from %q to %q", before, after)
		}
	})

	t.Run("a second dry run replaces the Plan", func(t *testing.T) {
		planPath := filepath.Join(t.TempDir(), "plan.json")
		for range 2 {
			got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, root)
			if got.status != 0 || got.stderr != "" {
				t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
			}
		}
		out := decodePlan(t, planPath)
		if paths := planPaths(t, out); !sameSet(t, paths, stale) {
			t.Errorf("Plan paths = %v, want %v", paths, stale)
		}
	})

	t.Run("sweep clean without --dry-run is a usage error", func(t *testing.T) {
		got := b.run(t, dir, "clean")
		if got.status != 2 {
			t.Fatalf("exit status %d, want 2", got.status)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want nothing", got.stdout)
		}
		if !strings.Contains(got.stderr, "--dry-run") {
			t.Errorf("stderr = %q, want the clean usage", got.stderr)
		}
		if _, err := os.Lstat(filepath.Join(dir, "sweep-plan.json")); !os.IsNotExist(err) {
			t.Errorf("wrote a Plan without --dry-run: %v", err)
		}
	})
}

func TestDryRunWritesNoPlanWhenAPickedCandidateCannotBeRead(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("only an account that is not root can be refused a directory by its mode")
	}
	b := build(t)
	top := t.TempDir()
	old := time.Now().Add(-60 * 24 * time.Hour)
	for _, rel := range []string{"app/package.json", "app/src/index.js", "app/node_modules/a/index.js", "app/node_modules/sealed/keep.js"} {
		p := filepath.Join(top, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	sealed := filepath.Join(top, "app", "node_modules", "sealed")
	if err := os.Chmod(sealed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o755) })

	planPath := filepath.Join(top, "plan.json")
	got := b.run(t, top, "clean", "--dry-run", "--plan", planPath, filepath.Join(top, "app"))
	if got.status != 1 {
		t.Fatalf("exit status %d, stderr %q, want 1", got.status, got.stderr)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want nothing when the Plan is not written", got.stdout)
	}
	if !strings.Contains(got.stderr, sealed) {
		t.Errorf("stderr = %q, want the directory that could not be read", got.stderr)
	}
	if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
		t.Errorf("wrote a Plan after a Fingerprint could not be read: %v", err)
	}
}

func TestDryRunFingerprintDoesNotFollowALink(t *testing.T) {
	b := build(t)
	top := t.TempDir()
	outside := filepath.Join(top, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "big.bin"), make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-60 * 24 * time.Hour)
	for _, rel := range []string{"app/package.json", "app/src/index.js", "app/node_modules/keep.txt"} {
		p := filepath.Join(top, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(top, "app", "node_modules", "escape")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
			t.Skipf("this account may not make a symlink: %v", err)
		}
		t.Fatal(err)
	}

	candidate := filepath.Join(top, "app", "node_modules")
	planPath := filepath.Join(top, "plan.json")
	got := b.run(t, top, "clean", "--dry-run", "--plan", planPath, filepath.Join(top, "app"))
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
	}
	out := decodePlan(t, planPath)
	if len(out.Candidates) != 1 || !sameDir(t, out.Candidates[0].Path, candidate) {
		t.Fatalf("Plan = %+v, want %s", out.Candidates, candidate)
	}
	c := out.Candidates[0]
	onDisk := du(t, candidate)
	if c.Fingerprint.Size != onDisk {
		t.Errorf("Size = %d, and du -s gives %d", c.Fingerprint.Size, onDisk)
	}
	entries, newest := census(t, candidate)
	if c.Fingerprint.Entries != entries {
		t.Errorf("entries = %d, and a listing that does not follow a Link gives %d", c.Fingerprint.Entries, entries)
	}
	if !c.Fingerprint.Newest.Equal(newest) {
		t.Errorf("newest = %s, and the listing's newest is %s", c.Fingerprint.Newest, newest)
	}
	if c.Fingerprint.Size >= 1<<20 {
		t.Errorf("Size = %d, which is the file the Link leads to", c.Fingerprint.Size)
	}
}

func TestDryRunRefusesAPlanReachedThroughALinkAboveItsParent(t *testing.T) {
	b := build(t)
	top := t.TempDir()
	old := time.Now().Add(-60 * 24 * time.Hour)
	for _, rel := range []string{"app/package.json", "app/src/index.js", "app/node_modules/keep.txt", "app/node_modules/subdir/keep.txt"} {
		p := filepath.Join(top, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	candidate := filepath.Join(top, "app", "node_modules")
	alias := filepath.Join(top, "app", "alias")
	if err := os.Symlink(candidate, alias); err != nil {
		if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
			t.Skipf("this account may not make a symlink: %v", err)
		}
		t.Fatal(err)
	}
	planPath := filepath.Join(alias, "subdir", "plan.json")
	got := b.run(t, top, "clean", "--dry-run", "--plan", planPath, filepath.Join(top, "app"))
	if got.status != 2 {
		t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
	}
	if _, err := os.Lstat(filepath.Join(candidate, "subdir", "plan.json")); !os.IsNotExist(err) {
		t.Errorf("wrote the Plan inside the Candidate through a Link: %v", err)
	}
}

func TestDryRunKeepsThePlanPathTheUserNamed(t *testing.T) {
	b := build(t)
	top := t.TempDir()
	old := time.Now().Add(-60 * 24 * time.Hour)
	for _, rel := range []string{"app/package.json", "app/src/index.js", "app/node_modules/keep.txt"} {
		p := filepath.Join(top, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(top, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(top, "alias")
	if err := os.Symlink(out, alias); err != nil {
		if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
			t.Skipf("this account may not make a symlink: %v", err)
		}
		t.Fatal(err)
	}
	planPath := filepath.Join(alias, "plan.json")
	got := b.run(t, top, "clean", "--dry-run", "--plan", planPath, filepath.Join(top, "app"))
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
	}
	if !strings.Contains(got.stdout, "Written to "+planPath) {
		t.Errorf("stdout is\n%s\nwant the path that was named", got.stdout)
	}
	if _, err := os.Lstat(planPath); err != nil {
		t.Errorf("the Plan was not written at the named path: %v", err)
	}
}

func TestDryRunRefusesAPlanAtAnotherSpellingOfACandidate(t *testing.T) {
	b := build(t)
	top := t.TempDir()
	old := time.Now().Add(-60 * 24 * time.Hour)
	for _, rel := range []string{"real/app/package.json", "real/app/src/index.js", "real/app/node_modules/keep.txt"} {
		p := filepath.Join(top, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	via := filepath.Join(top, "via")
	if err := os.Symlink(filepath.Join(top, "real"), via); err != nil {
		if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
			t.Skipf("this account may not make a symlink: %v", err)
		}
		t.Fatal(err)
	}
	planPath := filepath.Join(top, "real", "app", "node_modules", "plan.json")
	got := b.run(t, top, "clean", "--dry-run", "--plan", planPath, filepath.Join(via, "app"))
	if got.status != 2 {
		t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
	}
	if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
		t.Errorf("wrote the Plan inside the Candidate under another spelling: %v", err)
	}
}

type planJSON struct {
	Version    int      `json:"version"`
	Roots      []string `json:"roots"`
	Candidates []struct {
		Path        string `json:"path"`
		Fingerprint struct {
			Identity struct {
				Volume uint64 `json:"volume"`
				File   uint64 `json:"file"`
			} `json:"identity"`
			Entries int64     `json:"entries"`
			Size    int64     `json:"size"`
			Newest  time.Time `json:"newest"`
		} `json:"fingerprint"`
	} `json:"candidates"`
}

func decodePlan(t *testing.T, path string) planJSON {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out planJSON
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("the Plan is %s: %v", data, err)
	}
	if out.Version != 1 {
		t.Fatalf("version = %d, want 1", out.Version)
	}
	return out
}

func planPaths(t *testing.T, out planJSON) []string {
	t.Helper()
	var paths []string
	for _, c := range out.Candidates {
		paths = append(paths, c.Path)
	}
	return paths
}

func splitByAge(t *testing.T, key answerKey, at func(string) string) (stale, recent []string) {
	t.Helper()
	limit := time.Now().Add(-30 * 24 * time.Hour)
	for _, c := range key.Candidates {
		path := at(c.Path)
		used := newestOutside(t, filepath.Dir(path), path)
		if used.Before(limit) {
			stale = append(stale, path)
		} else {
			recent = append(recent, path)
		}
	}
	return stale, recent
}

func sameSet(t *testing.T, got, want []string) bool {
	t.Helper()
	if len(got) != len(want) {
		return false
	}
	used := make([]bool, len(want))
	for _, g := range got {
		found := false
		for i, w := range want {
			if !used[i] && sameDir(t, g, w) {
				used[i] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func assertSections(t *testing.T, stdout string, stale, recent []string) {
	t.Helper()
	pickedAt := strings.Index(stdout, "picked: Stale\n")
	leftAt := strings.Index(stdout, "not picked: used recently\n")
	if pickedAt < 0 || leftAt < pickedAt {
		t.Fatalf("stdout is\n%s\nwant picked: Stale and then not picked: used recently", stdout)
	}
	picked, left := stdout[pickedAt:leftAt], stdout[leftAt:]
	for _, path := range stale {
		if !strings.Contains(picked, path) || strings.Contains(left, path) {
			t.Errorf("%s should be under picked: Stale", path)
		}
	}
	for _, path := range recent {
		if !strings.Contains(left, path) || strings.Contains(picked, path) {
			t.Errorf("%s should be under not picked: used recently", path)
		}
	}
}

func census(t *testing.T, root string) (int64, time.Time) {
	t.Helper()
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	newest := info.ModTime()
	var entries int64
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		entries++
		inf, err := d.Info()
		if err != nil {
			return err
		}
		if inf.ModTime().After(newest) {
			newest = inf.ModTime()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries, newest
}

func countPhrase(n int) string {
	if n == 1 {
		return "1 Candidate"
	}
	return fmt.Sprintf("%d Candidates", n)
}

func shown(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	value, unit := float64(size)/1024, 0
	for value >= 1024 && unit < 3 {
		value, unit = value/1024, unit+1
	}
	return fmt.Sprintf("%.1f %s", value, [...]string{"KiB", "MiB", "GiB", "TiB"}[unit])
}
