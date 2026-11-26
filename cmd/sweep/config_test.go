package main_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestConfiguration(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.config = filepath.Join(dir, "config")
	if err := os.MkdirAll(b.config, 0o755); err != nil {
		t.Fatal(err)
	}
	projects := filepath.Join(dir, "projects")
	before := listing(t, projects)
	t.Cleanup(func() {
		for _, d := range differences(before, listing(t, projects)) {
			t.Errorf("the Fixture changed during a Scan: %s", d)
		}
	})
	elsewhere := t.TempDir()

	t.Run("a missing file means the current directory and the output says none", func(t *testing.T) {
		got := b.run(t, projects, "scan", "--json")
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		out := decode(t, got)
		if out.Configuration != "" {
			t.Errorf("configuration = %q, want none", out.Configuration)
		}
		if len(out.Roots) != 1 || !sameDir(t, out.Roots[0], projects) {
			t.Errorf("roots = %v, want %s", out.Roots, projects)
		}
		table := b.run(t, projects, "scan")
		if table.status != 0 || !strings.Contains(table.stdout, "Configuration\n  none\n") {
			t.Errorf("exit status %d, table:\n%s", table.status, table.stdout)
		}
	})

	t.Run("roots in the file are used from another directory and the output names the file", func(t *testing.T) {
		file := writeConfig(t, b.config, "roots = ["+tomlPath(t, projects)+"]\n")
		got := b.run(t, elsewhere, "scan", "--json")
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q, stdout %s", got.status, got.stderr, got.stdout)
		}
		out := decode(t, got)
		if out.Configuration != file {
			t.Errorf("configuration = %q, want %s", out.Configuration, file)
		}
		if len(out.Roots) != 1 || !sameDir(t, out.Roots[0], projects) {
			t.Errorf("roots = %v, want %s", out.Roots, projects)
		}
		table := b.run(t, elsewhere, "scan")
		if !strings.Contains(table.stdout, "Configuration\n  "+file+"\n") || !strings.Contains(table.stdout, "  "+out.Roots[0]+"\n") {
			t.Errorf("table:\n%s", table.stdout)
		}
	})

	t.Run("the command line wins", func(t *testing.T) {
		recent := filepath.Join(projects, "recent-app")
		writeConfig(t, b.config, "roots = ["+tomlPath(t, projects)+"]\n")
		got := b.run(t, elsewhere, "scan", "--json", recent)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		out := decode(t, got)
		if len(out.Roots) != 1 || !sameDir(t, out.Roots[0], recent) {
			t.Errorf("roots = %v, want %s", out.Roots, recent)
		}
		for _, c := range out.Candidates {
			if sameDir(t, c.Path, filepath.Join(projects, "web-app", "node_modules")) {
				t.Errorf("listed %s, and the Root was recent-app", c.Path)
			}
		}
	})

	t.Run("the stale threshold replaces 30 days", func(t *testing.T) {
		staleProject := filepath.Join(projects, "stale-app")
		staleCandidate := filepath.Join(staleProject, "node_modules")
		recentProject := filepath.Join(projects, "recent-app")
		recentCandidate := filepath.Join(recentProject, "node_modules")
		writeConfig(t, b.config, "roots = ["+tomlPath(t, projects)+"]\nstale_days = 90\n")
		got := b.run(t, elsewhere, "scan", "--json")
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		out := decode(t, got)
		if stale, want := staleness(t, out, staleCandidate), staleBefore(t, staleProject, staleCandidate, 90); stale != want {
			t.Errorf("stale-app Stale = %v, want %v from its modification time and 90 days", stale, want)
		}
		if stale, want := staleness(t, out, recentCandidate), staleBefore(t, recentProject, recentCandidate, 90); stale != want {
			t.Errorf("recent-app Stale = %v, want %v from its modification time and 90 days", stale, want)
		}

		writeConfig(t, b.config, "roots = ["+tomlPath(t, projects)+"]\nstale_days = 0\n")
		got = b.run(t, elsewhere, "scan", "--json")
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		out = decode(t, got)
		if stale, want := staleness(t, out, recentCandidate), staleBefore(t, recentProject, recentCandidate, 0); stale != want {
			t.Errorf("recent-app Stale = %v, want %v from its modification time and 0 days", stale, want)
		}
	})

	t.Run("an ignored path and everything beneath it is never reported", func(t *testing.T) {
		web := filepath.Join(projects, "web-app", "node_modules")
		stale := filepath.Join(projects, "stale-app")
		recent := filepath.Join(projects, "recent-app", "node_modules")
		writeConfig(t, b.config, "roots = ["+tomlPath(t, projects)+"]\nignore = ["+tomlPath(t, web)+", "+tomlPath(t, stale)+"]\n")
		sealed := filepath.Join(stale, "src")
		refused := runtime.GOOS != "windows" && os.Geteuid() != 0
		if refused {
			if err := os.Chmod(sealed, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(sealed, 0o755) })
		}
		got := b.run(t, elsewhere, "scan", "--json", projects)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q, stdout %s", got.status, got.stderr, got.stdout)
		}
		out := decode(t, got)
		hidden := []string{web, filepath.Join(stale, "node_modules")}
		for _, c := range out.Candidates {
			for _, path := range hidden {
				if sameDir(t, c.Path, path) {
					t.Errorf("listed %s, which is ignored", c.Path)
				}
			}
		}
		if !seen(t, out, recent) {
			t.Errorf("recent-app is not listed, and it was not ignored")
		}
		for _, want := range key.Candidates {
			path := filepath.Join(dir, filepath.FromSlash(want.Path))
			if ignoredPath(t, path, hidden) {
				continue
			}
			if !seen(t, out, path) {
				t.Errorf("the Answer key Candidate %s is not listed", want.Path)
			}
		}
		if refused {
			for _, u := range out.Unreadable {
				if sameDir(t, u.Path, sealed) {
					t.Errorf("reported %s, which is beneath an ignored path", u.Path)
				}
			}
		}
	})

	t.Run("a malformed file or an unknown key is an error that names the key", func(t *testing.T) {
		for name, body := range map[string]string{
			"syntax":   "roots = [\n",
			"days":     "stale_days = -1\n",
			"relative": "roots = ['projects']\n",
		} {
			t.Run(name, func(t *testing.T) {
				file := writeConfig(t, b.config, body)
				got := b.run(t, elsewhere, "scan", "--json")
				if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, file) {
					t.Fatalf("exit status %d, stderr %q, stdout %q", got.status, got.stderr, got.stdout)
				}
				key := ""
				if name == "days" {
					key = "stale_days"
				}
				if name == "relative" {
					key = "roots"
				}
				if key != "" && !strings.Contains(got.stderr, key) {
					t.Errorf("stderr = %q, want %s", got.stderr, key)
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
				got := b.run(t, elsewhere, "scan")
				if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, file) || !strings.Contains(got.stderr, `"`+key+`"`) {
					t.Fatalf("exit status %d, stderr %q, stdout %q, want status 2 naming %s and %s", got.status, got.stderr, got.stdout, file, key)
				}
			})
		}
	})
}

func TestAnIgnoredFileStillCountsForLastUsed(t *testing.T) {
	b := build(t)
	top := t.TempDir()
	b.config = filepath.Join(top, "config")
	app := filepath.Join(top, "app")
	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now().Add(-time.Hour)
	for _, rel := range []string{"package.json", "src/new.js", "node_modules/a/index.js"} {
		p := filepath.Join(app, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		when := old
		if rel == "src/new.js" {
			when = recent
		}
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatal(err)
		}
	}
	candidate := filepath.Join(app, "node_modules")
	src := filepath.Join(app, "src")
	want := newestOutside(t, app, candidate)
	wantStale := want.Before(time.Now().Add(-30 * 24 * time.Hour))

	one := func() scanJSON {
		t.Helper()
		got := b.run(t, top, "scan", "--json", app)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("exit status %d, stderr %q, stdout %s", got.status, got.stderr, got.stdout)
		}
		out := decode(t, got)
		if len(out.Candidates) != 1 || !sameDir(t, out.Candidates[0].Path, candidate) {
			t.Fatalf("candidates = %+v, want %s", out.Candidates, candidate)
		}
		return out
	}
	first := one()
	if !first.Candidates[0].LastUsed.Equal(want) || first.Candidates[0].Stale != wantStale {
		t.Fatalf("without Ignore, Last used = %s, Stale = %v, want %s and %v", first.Candidates[0].LastUsed, first.Candidates[0].Stale, want, wantStale)
	}

	writeConfig(t, b.config, "ignore = ["+tomlPath(t, src)+"]\n")
	second := one()
	if !second.Candidates[0].LastUsed.Equal(want) || second.Candidates[0].Stale != wantStale {
		t.Fatalf("with src ignored, Last used = %s, Stale = %v, want %s and %v", second.Candidates[0].LastUsed, second.Candidates[0].Stale, want, wantStale)
	}
	for _, u := range second.Unreadable {
		if sameDir(t, u.Path, src) || strings.HasPrefix(u.Path, src+string(filepath.Separator)) {
			t.Errorf("reported %s, which is ignored", u.Path)
		}
	}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return
	}
	sealed := filepath.Join(candidate, "sealed")
	if err := os.MkdirAll(sealed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sealed, "x.js"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sealed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o755) })
	writeConfig(t, b.config, "ignore = ["+tomlPath(t, src)+", "+tomlPath(t, sealed)+"]\n")
	got := b.run(t, top, "scan", "--json", app)
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit status %d, stderr %q, stdout %s", got.status, got.stderr, got.stdout)
	}
	out := decode(t, got)
	for _, u := range out.Unreadable {
		if sameDir(t, u.Path, sealed) {
			t.Errorf("reported %s, which is ignored", u.Path)
		}
	}
	if len(out.Candidates) != 1 || !out.Candidates[0].LastUsed.Equal(want) || out.Candidates[0].Stale != wantStale {
		t.Errorf("Last used = %s, Stale = %v, want %s and %v", out.Candidates[0].LastUsed, out.Candidates[0].Stale, want, wantStale)
	}
}

func writeConfig(t *testing.T, home, body string) string {
	t.Helper()
	path := configurationFile(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func tomlPath(t *testing.T, path string) string {
	t.Helper()
	if strings.Contains(path, "'") {
		t.Fatalf("path %s cannot be a TOML literal in this test", path)
	}
	return "'" + path + "'"
}

func staleness(t *testing.T, out scanJSON, path string) bool {
	t.Helper()
	for _, c := range out.Candidates {
		if sameDir(t, c.Path, path) {
			return c.Stale
		}
	}
	t.Fatalf("%s is not listed", path)
	return false
}

func staleBefore(t *testing.T, project, candidate string, days int) bool {
	t.Helper()
	used := newestOutside(t, project, candidate)
	return used.Before(time.Now().Add(-time.Duration(days) * 24 * time.Hour))
}

func ignoredPath(t *testing.T, path string, hidden []string) bool {
	t.Helper()
	for _, one := range hidden {
		if sameDir(t, path, one) {
			return true
		}
	}
	return false
}

func seen(t *testing.T, out scanJSON, path string) bool {
	t.Helper()
	for _, c := range out.Candidates {
		if sameDir(t, c.Path, path) {
			return true
		}
	}
	return false
}
