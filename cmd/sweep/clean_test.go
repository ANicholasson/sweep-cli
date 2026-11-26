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
	// The per-user cache is pointed into this run, as the configuration
	// directory is. The Plan written there is expected, so the comparison
	// below does not watch that directory.
	b.cache = filepath.Join(dir, "cache")
	if err := os.MkdirAll(b.cache, 0o755); err != nil {
		t.Fatal(err)
	}
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	root := at(key.Root)
	before := listing(t, dir)
	t.Cleanup(func() {
		for _, d := range differences(withoutTree(before, b.cache), withoutTree(listing(t, dir), b.cache)) {
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
			if candidateToo(key, trap.Path) {
				continue
			}
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

	t.Run("the default file is sweep/plan.json in the per-user cache directory", func(t *testing.T) {
		work := t.TempDir()
		got := b.run(t, work, "clean", "--dry-run", root)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		if _, err := os.Lstat(filepath.Join(work, "sweep-plan.json")); !os.IsNotExist(err) {
			t.Errorf("wrote sweep-plan.json in the working directory: %v", err)
		}
		planPath := planFile(b.cache)
		out := decodePlan(t, planPath)
		if paths := planPaths(t, out); !sameSet(t, paths, stale) {
			t.Errorf("Plan paths = %v, want %v", paths, stale)
		}
		written := writtenPath(got.stdout)
		if !filepath.IsAbs(written) {
			t.Errorf("Written to %q, want a full path", written)
		}
		if !sameFileNamed(t, got.stdout, planPath) {
			t.Errorf("stdout is\n%s\nwant the default path %s", got.stdout, planPath)
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

	t.Run("a dry run from inside a Candidate writes the Plan in the cache", func(t *testing.T) {
		candidate := at("projects/stale-app/node_modules")
		got := b.run(t, candidate, "clean", "--dry-run", root)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		if _, err := os.Lstat(filepath.Join(candidate, "sweep-plan.json")); !os.IsNotExist(err) {
			t.Errorf("wrote sweep-plan.json inside the Candidate: %v", err)
		}
		if _, err := os.Lstat(planFile(b.cache)); err != nil {
			t.Errorf("the Plan was not written in the cache: %v", err)
		}
	})

	t.Run("a Plan path inside a Project is refused before anything is written", func(t *testing.T) {
		project := at("projects/stale-app")
		planPath := filepath.Join(project, "src", "plan.json")
		got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, root)
		if got.status != 2 {
			t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want nothing", got.stdout)
		}
		if !strings.Contains(got.stderr, planPath) || !strings.Contains(got.stderr, project) {
			t.Errorf("stderr = %q, want the path and the Project", got.stderr)
		}
		if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
			t.Errorf("wrote the Plan inside the Project: %v", err)
		}
	})

	t.Run("a Plan reached through a Link into a Project is refused", func(t *testing.T) {
		project := at("projects/stale-app")
		link := filepath.Join(t.TempDir(), "project")
		if err := os.Symlink(project, link); err != nil {
			if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
				t.Skipf("this account may not make a symlink: %v", err)
			}
			t.Fatal(err)
		}
		planPath := filepath.Join(link, "plan.json")
		got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, root)
		if got.status != 2 {
			t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want nothing", got.stdout)
		}
		if !strings.Contains(got.stderr, planPath) || !strings.Contains(got.stderr, project) {
			t.Errorf("stderr = %q, want the path and the Project", got.stderr)
		}
		if _, err := os.Lstat(filepath.Join(project, "plan.json")); !os.IsNotExist(err) {
			t.Errorf("wrote the Plan inside the Project through a Link: %v", err)
		}
	})

	t.Run("a Plan path in a missing directory inside a Project is refused", func(t *testing.T) {
		project := at("projects/stale-app")
		planPath := filepath.Join(project, "missing", "plan.json")
		got := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, root)
		if got.status != 2 {
			t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want nothing", got.stdout)
		}
		if !strings.Contains(got.stderr, planPath) || !strings.Contains(got.stderr, project) {
			t.Errorf("stderr = %q, want the path and the Project", got.stderr)
		}
		if _, err := os.Lstat(filepath.Join(project, "missing")); !os.IsNotExist(err) {
			t.Errorf("created a directory inside the Project: %v", err)
		}
	})

	t.Run("a relative Plan path inside a Stale Project is refused", func(t *testing.T) {
		project := at("projects/stale-app")
		got := b.run(t, project, "clean", "--dry-run", "--plan", "plan.json", ".")
		if got.status != 2 {
			t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want nothing", got.stdout)
		}
		if !strings.Contains(got.stderr, "plan.json") || !strings.Contains(got.stderr, project) {
			t.Errorf("stderr = %q, want the path and the Project", got.stderr)
		}
		if _, err := os.Lstat(filepath.Join(project, "plan.json")); !os.IsNotExist(err) {
			t.Errorf("wrote the Plan inside the Project: %v", err)
		}
	})

	t.Run("two dry runs from inside a Stale Project give the same Plan", func(t *testing.T) {
		project := at("projects/stale-app")
		var want []string
		for _, path := range stale {
			if path == project || strings.HasPrefix(path, project+string(os.PathSeparator)) {
				want = append(want, path)
			}
		}
		if len(want) != 1 {
			t.Fatalf("Stale Candidates in %s = %d, want 1", project, len(want))
		}
		before := listing(t, project)
		planPath := planFile(b.cache)
		var first []byte
		var last ran
		for i := range 2 {
			got := b.run(t, project, "clean", "--dry-run", ".")
			last = got
			if got.status != 0 || got.stderr != "" {
				t.Fatalf("run %d: exit status %d, stderr %q", i+1, got.status, got.stderr)
			}
			if !strings.Contains(got.stdout, countPhrase(len(want))+" in the Plan") {
				t.Errorf("run %d stdout is\n%s\nwant %s in the Plan", i+1, got.stdout, countPhrase(len(want)))
			}
			data, err := os.ReadFile(planPath)
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				first = data
			} else if string(data) != string(first) {
				t.Errorf("the second Plan is\n%s\nthe first is\n%s", data, first)
			}
		}
		out := decodePlan(t, planPath)
		if paths := planPaths(t, out); !sameSet(t, paths, want) {
			t.Errorf("Plan paths = %v, want %v", paths, want)
		}
		for _, c := range out.Candidates {
			onDisk := du(t, c.Path)
			if c.Fingerprint.Size != onDisk {
				t.Errorf("%s: Fingerprint Size = %d, and du -s gives %d", c.Path, c.Fingerprint.Size, onDisk)
			}
		}
		written := writtenPath(last.stdout)
		if !filepath.IsAbs(written) || !sameFileNamed(t, last.stdout, planPath) {
			t.Errorf("stdout is\n%s\nwant the full path %s", last.stdout, planPath)
		}
		for _, d := range differences(before, listing(t, project)) {
			t.Errorf("the Project changed: %s", d)
		}
	})

	t.Run("the default Plan is refused when the cache directory is inside a Project", func(t *testing.T) {
		project := at("projects/stale-app")
		local := b
		local.cache = project
		got := local.run(t, dir, "clean", "--dry-run", root)
		if got.status != 2 {
			t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("stdout = %q, want nothing", got.stdout)
		}
		if !strings.Contains(got.stderr, project) || !strings.Contains(got.stderr, "plan.json") {
			t.Errorf("stderr = %q, want the path and the Project", got.stderr)
		}
		for _, rel := range []string{"sweep", "Library"} {
			if _, err := os.Lstat(filepath.Join(project, rel)); !os.IsNotExist(err) {
				t.Errorf("wrote %s inside the Project: %v", rel, err)
			}
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
		manifest := filepath.Join(dir, "answer-key.json")
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

func TestDryRunReadsTheConfiguration(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.config = filepath.Join(dir, "config")
	if err := os.MkdirAll(b.config, 0o755); err != nil {
		t.Fatal(err)
	}
	projects := filepath.Join(dir, "projects")
	elsewhere := t.TempDir()
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	staleCandidate := at("projects/stale-app/node_modules")
	recentCandidate := at("projects/recent-app/node_modules")
	webCandidate := at("projects/web-app/node_modules")

	// The snapshot is taken after the subtest has written its configuration,
	// so the assertion is that the dry run left the Fixture as it stood.
	unchanged := func(t *testing.T) func() {
		t.Helper()
		before := listing(t, dir)
		return func() {
			t.Helper()
			for _, d := range differences(before, listing(t, dir)) {
				t.Errorf("the Fixture changed during a dry run: %s", d)
			}
		}
	}

	t.Run("a missing file uses the current directory and the output says none", func(t *testing.T) {
		saved := b.config
		b.config = t.TempDir()
		defer func() { b.config = saved }()
		defer unchanged(t)()
		planPath := filepath.Join(t.TempDir(), "plan.json")
		got := b.run(t, projects, "clean", "--dry-run", "--plan", planPath)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		if !strings.Contains(got.stdout, "Configuration\n  none\n") {
			t.Errorf("stdout is\n%s\nwant Configuration none", got.stdout)
		}
		out := decodePlan(t, planPath)
		if roots := out.Roots; len(roots) != 1 || !sameDir(t, roots[0], projects) {
			t.Errorf("roots = %v, want %s", roots, projects)
		}
	})

	t.Run("roots in the file are used in that order and the output names the file", func(t *testing.T) {
		first := at("projects/recent-app")
		second := at("projects/web-app")
		file := writeConfig(t, b.config, "roots = ["+tomlPath(t, first)+", "+tomlPath(t, second)+"]\n")
		defer unchanged(t)()
		planPath := filepath.Join(t.TempDir(), "plan.json")
		got := b.run(t, elsewhere, "clean", "--dry-run", "--plan", planPath)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		if !strings.Contains(got.stdout, "Configuration\n  "+file+"\n") {
			t.Errorf("stdout is\n%s\nwant the file %s", got.stdout, file)
		}
		out := decodePlan(t, planPath)
		if len(out.Roots) != 2 || !sameDir(t, out.Roots[0], first) || !sameDir(t, out.Roots[1], second) {
			t.Errorf("roots = %v, want %s then %s", out.Roots, first, second)
		}
		scanned := b.run(t, elsewhere, "scan", "--json")
		if scanned.status != 0 || scanned.stderr != "" {
			t.Fatalf("scan exit status %d, stderr %q", scanned.status, scanned.stderr)
		}
		report := decode(t, scanned)
		if len(report.Roots) != 2 || !sameDir(t, report.Roots[0], first) || !sameDir(t, report.Roots[1], second) {
			t.Errorf("scan roots = %v, want %s then %s", report.Roots, first, second)
		}
	})

	t.Run("the command line wins, in the order given", func(t *testing.T) {
		fileFirst := at("projects/web-app")
		fileSecond := at("projects/recent-app")
		cliFirst := at("projects/stale-app")
		cliSecond := at("projects/recent-app")
		writeConfig(t, b.config, "roots = ["+tomlPath(t, fileFirst)+", "+tomlPath(t, fileSecond)+"]\n")
		defer unchanged(t)()
		planPath := filepath.Join(t.TempDir(), "plan.json")
		got := b.run(t, elsewhere, "clean", "--dry-run", "--plan", planPath, cliFirst, cliSecond)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		out := decodePlan(t, planPath)
		if len(out.Roots) != 2 || !sameDir(t, out.Roots[0], cliFirst) || !sameDir(t, out.Roots[1], cliSecond) {
			t.Errorf("roots = %v, want %s then %s", out.Roots, cliFirst, cliSecond)
		}
		scanned := b.run(t, elsewhere, "scan", "--json", cliFirst, cliSecond)
		if scanned.status != 0 || scanned.stderr != "" {
			t.Fatalf("scan exit status %d, stderr %q", scanned.status, scanned.stderr)
		}
		report := decode(t, scanned)
		if len(report.Roots) != 2 || !sameDir(t, report.Roots[0], cliFirst) || !sameDir(t, report.Roots[1], cliSecond) {
			t.Errorf("scan roots = %v, want %s then %s", report.Roots, cliFirst, cliSecond)
		}
	})

	t.Run("the current directory is the Root when the file names none", func(t *testing.T) {
		file := writeConfig(t, b.config, "stale_days = 30\n")
		defer unchanged(t)()
		planPath := filepath.Join(t.TempDir(), "plan.json")
		got := b.run(t, projects, "clean", "--dry-run", "--plan", planPath)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		if !strings.Contains(got.stdout, "Configuration\n  "+file+"\n") {
			t.Errorf("stdout is\n%s\nwant the file %s", got.stdout, file)
		}
		out := decodePlan(t, planPath)
		if len(out.Roots) != 1 || !sameDir(t, out.Roots[0], projects) {
			t.Errorf("roots = %v, want %s", out.Roots, projects)
		}
	})

	t.Run("the stale threshold replaces 30 days", func(t *testing.T) {
		for _, days := range []int{90, 0} {
			t.Run(fmt.Sprintf("%d days", days), func(t *testing.T) {
				writeConfig(t, b.config, "roots = ["+tomlPath(t, projects)+"]\nstale_days = "+fmt.Sprint(days)+"\n")
				defer unchanged(t)()
				planPath := filepath.Join(t.TempDir(), "plan.json")
				got := b.run(t, elsewhere, "clean", "--dry-run", "--plan", planPath)
				if got.status != 0 || got.stderr != "" {
					t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
				}
				out := decodePlan(t, planPath)
				var want []string
				for _, c := range key.Candidates {
					path := at(c.Path)
					if staleBefore(t, filepath.Dir(path), path, days) {
						want = append(want, path)
					}
				}
				if paths := planPaths(t, out); !sameSet(t, paths, want) {
					t.Errorf("Plan paths = %v, want the Candidates whose Last used is before %d days ago: %v", paths, days, want)
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
				for _, path := range []string{staleCandidate, recentCandidate} {
					held := planHolds(t, out, path)
					if held != staleBefore(t, filepath.Dir(path), path, days) {
						t.Errorf("%s in the Plan = %v, and its modification time with %d days says otherwise", path, held, days)
					}
				}
			})
		}
	})

	t.Run("an ignored path and everything beneath it is never in the Plan", func(t *testing.T) {
		staleProject := at("projects/stale-app")
		file := writeConfig(t, b.config, "roots = ["+tomlPath(t, projects)+"]\nignore = ["+tomlPath(t, webCandidate)+", "+tomlPath(t, staleProject)+"]\n")
		defer unchanged(t)()
		hidden := []string{webCandidate, staleCandidate}
		planPath := filepath.Join(t.TempDir(), "plan.json")
		got := b.run(t, elsewhere, "clean", "--dry-run", "--plan", planPath, projects)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		if !strings.Contains(got.stdout, "Configuration\n  "+file+"\n") {
			t.Errorf("stdout is\n%s\nwant the file %s", got.stdout, file)
		}
		out := decodePlan(t, planPath)
		for _, path := range hidden {
			if planHolds(t, out, path) {
				t.Errorf("the Plan holds %s, which is ignored", path)
			}
			if strings.Contains(got.stdout, path) {
				t.Errorf("stdout names %s, which is ignored", path)
			}
		}
		if !strings.Contains(got.stdout, recentCandidate) {
			t.Errorf("stdout leaves out %s, and it was not ignored", recentCandidate)
		}
		scanned := b.run(t, elsewhere, "scan", "--json", projects)
		if scanned.status != 0 || scanned.stderr != "" {
			t.Fatalf("scan exit status %d, stderr %q", scanned.status, scanned.stderr)
		}
		report := decode(t, scanned)
		for _, c := range report.Candidates {
			for _, path := range hidden {
				if sameDir(t, c.Path, path) {
					t.Errorf("the Scan listed %s, which is ignored", c.Path)
				}
			}
		}

		for _, path := range hidden {
			t.Run("pick "+filepath.Base(filepath.Dir(path)), func(t *testing.T) {
				defer unchanged(t)()
				picked := filepath.Join(t.TempDir(), "plan.json")
				got := b.run(t, elsewhere, "clean", "--dry-run", "--plan", picked, "--pick", path, projects)
				if got.status != 2 || got.stdout != "" {
					t.Fatalf("exit status %d, stdout %q, stderr %q, want 2 and no Plan", got.status, got.stdout, got.stderr)
				}
				if !strings.Contains(got.stderr, path) {
					t.Errorf("stderr = %q, want it to name %s", got.stderr, path)
				}
				if _, err := os.Lstat(picked); !os.IsNotExist(err) {
					t.Errorf("wrote a Plan for an ignored path: %v", err)
				}
			})
		}
	})

	t.Run("no key switches off a safety rule or makes delete the default", func(t *testing.T) {
		for _, key := range []string{"git", "root", "fingerprint", "delete"} {
			t.Run(key, func(t *testing.T) {
				body := key + " = false\n"
				if key == "delete" {
					body = "delete = true\n"
				}
				file := writeConfig(t, b.config, body)
				defer unchanged(t)()
				planPath := filepath.Join(t.TempDir(), "plan.json")
				got := b.run(t, projects, "clean", "--dry-run", "--plan", planPath)
				if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, file) || !strings.Contains(got.stderr, `"`+key+`"`) {
					t.Fatalf("exit status %d, stderr %q, stdout %q, want status 2 naming %s and %s", got.status, got.stderr, got.stdout, file, key)
				}
				if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
					t.Errorf("wrote a Plan after rejecting the configuration: %v", err)
				}
			})
		}
	})
}

func TestACandidateThatHoldsAnIgnorePathIsLeftOutOfThePlan(t *testing.T) {
	b := build(t)
	top := t.TempDir()
	b.config = filepath.Join(top, "config")
	writeOldProject(t, top)
	keep := filepath.Join(top, "app", "node_modules", "keep")
	candidate := filepath.Join(top, "app", "node_modules")
	other := filepath.Join(top, "other", "node_modules")
	writeConfig(t, b.config, "ignore = ["+tomlPath(t, keep)+"]\n")

	planPath := filepath.Join(t.TempDir(), "plan.json")
	got := b.run(t, top, "clean", "--dry-run", "--plan", planPath, top)
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
	}
	out := decodePlan(t, planPath)
	if planHolds(t, out, candidate) {
		t.Errorf("the Plan holds %s, which holds an Ignore path", candidate)
	}
	if strings.Contains(got.stdout, candidate) {
		t.Errorf("stdout names %s, which holds an Ignore path", candidate)
	}
	if !planHolds(t, out, other) {
		t.Errorf("the Plan leaves out %s", other)
	}
	for _, c := range out.Candidates {
		if c.Fingerprint.Size != du(t, c.Path) {
			t.Errorf("%s: Fingerprint Size = %d, and du -s gives %d", c.Path, c.Fingerprint.Size, du(t, c.Path))
		}
	}
	if _, err := os.Lstat(filepath.Join(keep, "precious.txt")); err != nil {
		t.Errorf("the ignored file is gone: %v", err)
	}

	picked := filepath.Join(t.TempDir(), "picked.json")
	got = b.run(t, top, "clean", "--dry-run", "--plan", picked, "--pick", candidate, top)
	if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, candidate) {
		t.Fatalf("exit status %d, stdout %q, stderr %q, want 2 naming %s", got.status, got.stdout, got.stderr, candidate)
	}
	if _, err := os.Lstat(picked); !os.IsNotExist(err) {
		t.Errorf("wrote a Plan for a Candidate that holds an Ignore path: %v", err)
	}
}

func TestAPlanInsideADroppedCandidateOrItsProjectIsRefused(t *testing.T) {
	b := build(t)
	top := t.TempDir()
	b.config = filepath.Join(top, "config")
	writeOldProject(t, top)
	keep := filepath.Join(top, "app", "node_modules", "keep")
	project := filepath.Join(top, "app")
	candidate := filepath.Join(top, "app", "node_modules")
	writeConfig(t, b.config, "ignore = ["+tomlPath(t, keep)+"]\n")

	t.Run("the Project", func(t *testing.T) {
		planPath := filepath.Join(project, "plan.json")
		got := b.run(t, top, "clean", "--dry-run", "--plan", planPath, top)
		if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, planPath) || !strings.Contains(got.stderr, project) {
			t.Fatalf("exit status %d, stdout %q, stderr %q, want 2 naming the Plan and the Project", got.status, got.stdout, got.stderr)
		}
		if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
			t.Errorf("wrote the Plan inside the Project: %v", err)
		}
	})
	t.Run("the Candidate", func(t *testing.T) {
		planPath := filepath.Join(candidate, "plan.json")
		got := b.run(t, top, "clean", "--dry-run", "--plan", planPath, top)
		if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, candidate) {
			t.Fatalf("exit status %d, stdout %q, stderr %q, want 2 naming the Candidate", got.status, got.stdout, got.stderr)
		}
		if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
			t.Errorf("wrote the Plan inside the Candidate: %v", err)
		}
	})
}

func TestAPlanInsideAnIgnorePathIsRefused(t *testing.T) {
	b := build(t)
	top := t.TempDir()
	b.config = filepath.Join(top, "config")
	writeOldProject(t, top)

	t.Run("an ignored Project", func(t *testing.T) {
		project := filepath.Join(top, "app")
		writeConfig(t, b.config, "ignore = ["+tomlPath(t, project)+"]\n")
		planPath := filepath.Join(project, "plan.json")
		got := b.run(t, top, "clean", "--dry-run", "--plan", planPath, top)
		if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, planPath) || !strings.Contains(got.stderr, project) {
			t.Fatalf("exit status %d, stdout %q, stderr %q, want 2 naming the Plan and the Project", got.status, got.stdout, got.stderr)
		}
		if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
			t.Errorf("wrote the Plan inside the ignored Project: %v", err)
		}
	})

	t.Run("an ignored Candidate", func(t *testing.T) {
		candidate := filepath.Join(top, "other", "node_modules")
		writeConfig(t, b.config, "ignore = ["+tomlPath(t, candidate)+"]\n")
		planPath := filepath.Join(candidate, "plan.json")
		got := b.run(t, top, "clean", "--dry-run", "--plan", planPath, top)
		if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, planPath) || !strings.Contains(got.stderr, candidate) {
			t.Fatalf("exit status %d, stdout %q, stderr %q, want 2 naming the Plan and the Candidate", got.status, got.stdout, got.stderr)
		}
		if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
			t.Errorf("wrote the Plan inside the ignored Candidate: %v", err)
		}
	})
}

func writeOldProject(t *testing.T, top string) {
	t.Helper()
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, rel := range []string{
		"app/package.json",
		"app/src/a.js",
		"app/node_modules/keep/precious.txt",
		"other/package.json",
		"other/node_modules/a.js",
	} {
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

func planHolds(t *testing.T, out planJSON, path string) bool {
	t.Helper()
	for _, c := range out.Candidates {
		if sameDir(t, c.Path, path) {
			return true
		}
	}
	return false
}

func candidateToo(key answerKey, path string) bool {
	for _, c := range key.Candidates {
		if c.Path == path {
			return true
		}
	}
	return false
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

// withoutTree drops skip and everything under it, so the per-user cache
// pointed into the run is not counted as a change to the Fixture.
func withoutTree(found map[string]string, skip string) map[string]string {
	prefix := skip + string(os.PathSeparator)
	out := make(map[string]string, len(found))
	for p, line := range found {
		if p == skip || strings.HasPrefix(p, prefix) {
			continue
		}
		out[p] = line
	}
	return out
}

func writtenPath(stdout string) string {
	for _, line := range strings.Split(stdout, "\n") {
		if rest, ok := strings.CutPrefix(line, "Written to "); ok {
			return rest
		}
	}
	return ""
}

// sameFileNamed reports whether stdout's "Written to" line names path.
// On macOS the working directory may be spelled /var or /private/var.
func sameFileNamed(t *testing.T, stdout, path string) bool {
	t.Helper()
	var named string
	for _, line := range strings.Split(stdout, "\n") {
		if rest, ok := strings.CutPrefix(line, "Written to "); ok {
			named = rest
		}
	}
	if named == "" {
		return false
	}
	a, err := os.Stat(named)
	if err != nil {
		return false
	}
	b, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(a, b)
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
