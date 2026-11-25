package main_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

type scanJSON struct {
	Version    int      `json:"version"`
	Roots      []string `json:"roots"`
	Candidates []struct {
		Path      string    `json:"path"`
		Size      int64     `json:"size"`
		Kind      string    `json:"kind"`
		Ecosystem string    `json:"ecosystem"`
		Reason    string    `json:"reason"`
		Project   string    `json:"project"`
		LastUsed  time.Time `json:"last_used"`
		Stale     bool      `json:"stale"`
	} `json:"candidates"`
	Unreadable []struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	} `json:"unreadable"`
	DockerData *struct {
		Size *int64 `json:"size"`
		Mark string `json:"mark"`
	} `json:"dockerData"`
}

func decode(t *testing.T, got ran) scanJSON {
	t.Helper()
	var out scanJSON
	decoder := json.NewDecoder(strings.NewReader(got.stdout))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		t.Fatalf("sweep scan --json printed %q: %v", got.stdout, err)
	}
	if out.Version != 1 {
		t.Fatalf("version = %d, want 1", out.Version)
	}
	return out
}

func TestScanOfTheFixture(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	root := at(key.Root)
	before := listing(t, dir)
	t.Cleanup(func() {
		for _, d := range differences(before, listing(t, dir)) {
			t.Errorf("the Fixture changed during a Scan: %s", d)
		}
	})

	scan := func(t *testing.T, in string, roots ...string) scanJSON {
		t.Helper()
		got := b.run(t, in, append([]string{"scan", "--json"}, roots...)...)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("sweep scan --json %v: exit status %d, stderr %q", roots, got.status, got.stderr)
		}
		return decode(t, got)
	}
	listed := func(out scanJSON, path string) int {
		n := 0
		for _, c := range out.Candidates {
			if sameDir(t, c.Path, path) {
				n++
			}
		}
		return n
	}
	rootsAre := func(t *testing.T, out scanJSON, want []string) {
		t.Helper()
		if len(out.Roots) != len(want) {
			t.Fatalf("roots = %v, want %v", out.Roots, want)
		}
		for i := range want {
			if !sameDir(t, out.Roots[i], want[i]) {
				t.Errorf("roots = %v, want %v", out.Roots, want)
			}
		}
	}
	nothingInsideACandidate := func(t *testing.T, out scanJSON) {
		t.Helper()
		for _, outer := range out.Candidates {
			err := filepath.WalkDir(outer.Path, func(p string, d fs.DirEntry, err error) error {
				if err != nil || p == outer.Path || !d.IsDir() {
					return err
				}
				for _, inner := range out.Candidates {
					if sameDir(t, inner.Path, p) {
						t.Errorf("%s is listed, and it is inside the Candidate %s", inner.Path, outer.Path)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Run("every Candidate in the Answer key is listed once, with its Kind, a Reason and the Size du gives", func(t *testing.T) {
		for _, want := range key.Candidates {
			var out scanJSON
			onDisk := settled(t, at(want.Path), func() { out = scan(t, dir, root) })
			if n := listed(out, at(want.Path)); n != 1 {
				t.Errorf("%s is listed %d times, want once", want.Path, n)
				continue
			}
			for _, got := range out.Candidates {
				if !sameDir(t, got.Path, at(want.Path)) {
					continue
				}
				if got.Kind != want.Kind {
					t.Errorf("%s: Kind = %q, want %q", want.Path, got.Kind, want.Kind)
				}
				if got.Size != onDisk {
					t.Errorf("%s: Size = %d bytes, and du -s gives %d", want.Path, got.Size, onDisk)
				}
				if !strings.HasSuffix(got.Reason, ".") || got.Ecosystem == "" {
					t.Errorf("%s: Reason = %q, Ecosystem = %q, want a sentence and a Name", want.Path, got.Reason, got.Ecosystem)
				}
			}
		}
	})

	t.Run("nothing inside a Candidate is listed", func(t *testing.T) {
		nested := 0
		for _, c := range key.Candidates {
			err := filepath.WalkDir(at(c.Path), func(p string, d fs.DirEntry, err error) error {
				if err == nil && p != at(c.Path) && d.IsDir() && d.Name() == filepath.Base(at(c.Path)) {
					nested++
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		if nested == 0 {
			t.Fatal("no Candidate of the Fixture holds a directory of its own name, so this shows nothing")
		}
		nothingInsideACandidate(t, scan(t, dir, root))
	})

	t.Run("no Trap is listed", func(t *testing.T) {
		out := scan(t, dir, root)
		if len(key.Traps) == 0 {
			t.Fatal("the Answer key has no Trap, so this shows nothing")
		}
		for _, trap := range key.Traps {
			if listed(out, at(trap.Path)) != 0 {
				t.Errorf("the Trap %s is listed", trap.Path)
			}
		}
	})

	t.Run("Roots that overlap list each Candidate once and nothing inside one, whatever their order", func(t *testing.T) {
		for _, c := range key.Candidates {
			candidate := at(c.Path)
			project := filepath.Dir(candidate)
			for _, roots := range [][]string{
				{root, root},
				{project, root},
				{root, project},
				{candidate, root},
				{root, candidate},
			} {
				out := scan(t, dir, roots...)
				rootsAre(t, out, roots)
				for _, want := range key.Candidates {
					if n := listed(out, at(want.Path)); n != 1 {
						t.Errorf("Roots %v: %s is listed %d times, want once", roots, want.Path, n)
					}
				}
				nothingInsideACandidate(t, out)
			}
		}
	})

	t.Run("with no Root the current directory is the Root", func(t *testing.T) {
		out := scan(t, root)
		rootsAre(t, out, []string{root})
		for _, want := range key.Candidates {
			if listed(out, at(want.Path)) != 1 {
				t.Errorf("%s is not listed once", want.Path)
			}
		}
	})

	t.Run("a flag may come after the Roots", func(t *testing.T) {
		got := b.run(t, dir, "scan", root, "--json")
		if got.status != 0 {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		rootsAre(t, decode(t, got), []string{root})
	})

	t.Run("the table names the Roots, and each Candidate with its Size, Kind and Reason", func(t *testing.T) {
		out := scan(t, dir, root)
		got := b.run(t, dir, "scan", root)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("sweep scan: exit status %d, stderr %q", got.status, got.stderr)
		}
		roots, rows := readTable(t, got.stdout)
		if len(roots) != 1 || !sameDir(t, roots[0], root) {
			t.Errorf("the table names Roots %v, want %s", roots, root)
		}
		if len(rows) != len(out.Candidates) {
			t.Errorf("the table has %d rows and the JSON %d Candidates", len(rows), len(out.Candidates))
		}
		for _, want := range key.Candidates {
			i := slices.IndexFunc(rows, func(r row) bool { return sameDir(t, r.path, at(want.Path)) })
			if i < 0 {
				t.Errorf("%s is not in the table:\n%s", want.Path, got.stdout)
				continue
			}
			r := rows[i]
			onDisk := du(t, at(want.Path))
			if math.Abs(float64(onDisk)-r.size) > r.within {
				t.Errorf("%s: the table gives %q, and du -s gives %d bytes", want.Path, r.cells[0], onDisk)
			}
			if r.kind != want.Kind || !strings.HasSuffix(r.reason, ".") {
				t.Errorf("%s: Kind %q and Reason %q, want %q and a sentence", want.Path, r.kind, r.reason, want.Kind)
			}
		}
	})

	t.Run("each Candidate is grouped by Project, and Stale follows Last used", func(t *testing.T) {
		out := scan(t, dir, root)
		table := b.run(t, dir, "scan", root)
		if table.status != 0 || table.stderr != "" {
			t.Fatalf("sweep scan: exit status %d, stderr %q", table.status, table.stderr)
		}
		for _, c := range out.Candidates {
			project := filepath.Dir(c.Path)
			manifest := filepath.Join(project, "package.json")
			info, err := os.Lstat(manifest)
			if err != nil || !info.Mode().IsRegular() {
				t.Errorf("%s: Project %s has no manifest", c.Path, c.Project)
			}
			if !sameDir(t, c.Project, project) {
				t.Errorf("%s: Project = %s, want %s", c.Path, c.Project, project)
			}
			used := newestOutside(t, project, c.Path)
			if !c.LastUsed.Equal(used) {
				t.Errorf("%s: Last used = %s, and the newest file outside it is %s", c.Path, c.LastUsed, used)
			}
			stale := used.Before(time.Now().Add(-30 * 24 * time.Hour))
			if c.Stale != stale {
				t.Errorf("%s: Stale = %v, want %v from that file's modification time", c.Path, c.Stale, stale)
			}
			if !strings.Contains(table.stdout, "\n"+c.Project+"\n") {
				t.Errorf("the table does not group %s under %s\n%s", c.Path, c.Project, table.stdout)
			}
			row := rowFor(t, table.stdout, c.Path)
			if strings.Contains(row, " stale ") != c.Stale {
				t.Errorf("the row %q marks Stale %v, and the JSON says %v", row, strings.Contains(row, " stale "), c.Stale)
			}
		}
		var stale, recent bool
		for _, c := range out.Candidates {
			switch {
			case strings.HasSuffix(filepath.ToSlash(c.Path), "projects/stale-app/node_modules"):
				stale = c.Stale
			case strings.HasSuffix(filepath.ToSlash(c.Path), "projects/recent-app/node_modules"):
				recent = !c.Stale
			}
		}
		if !stale || !recent {
			t.Errorf("stale-app Stale = %v, recent-app not Stale = %v, want the Fixture's pair", stale, recent)
		}
	})

	t.Run("a Scan that finds nothing writes empty lists, not null", func(t *testing.T) {
		got := b.run(t, dir, "scan", "--json", t.TempDir())
		if got.status != 0 {
			t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(got.stdout), &fields); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"candidates", "unreadable"} {
			if string(fields[name]) != "[]" {
				t.Errorf("%s = %s, want []", name, fields[name])
			}
		}
	})

	t.Run("a Root that is not a directory stops the Scan before it starts", func(t *testing.T) {
		for _, bad := range []string{at("gone"), at("answer-key.json")} {
			got := b.run(t, dir, "scan", "--json", root, bad)
			if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, bad) {
				t.Errorf("Root %s: exit status %d, stdout %q, stderr %q, want 2, nothing, and the Root named", bad, got.status, got.stdout, got.stderr)
			}
		}
	})

	t.Run("Docker data is its own field and not a Candidate", func(t *testing.T) {
		out := scan(t, dir, root)
		if out.DockerData == nil || out.DockerData.Mark == "" {
			t.Fatalf("dockerData = %+v, want a mark", out.DockerData)
		}
		for _, c := range out.Candidates {
			if c.Kind == "Docker data" || c.Reason == "reported only" {
				t.Errorf("candidate %+v is Docker data", c)
			}
		}
		table := b.run(t, dir, "scan", root)
		if table.status != 0 || table.stderr != "" {
			t.Fatalf("sweep scan: exit status %d, stderr %q", table.status, table.stderr)
		}
		if _, rows := readTable(t, table.stdout); len(rows) != len(out.Candidates) {
			t.Errorf("the table has %d rows and the JSON %d Candidates", len(rows), len(out.Candidates))
		}
		df, err := exec.Command("docker", "system", "df", "--format", "{{json .}}").Output()
		if err != nil {
			if out.DockerData.Size != nil {
				t.Errorf("docker system df failed (%v) but the Scan gives a size", err)
			}
			switch out.DockerData.Mark {
			case "docker is missing", "docker is not running", "docker did not answer":
			default:
				t.Errorf("mark = %q, want why Docker data was not read", out.DockerData.Mark)
			}
			if !strings.Contains(table.stdout, "\nDocker data  "+out.DockerData.Mark+"\n") {
				t.Errorf("table is\n%s\nwant the Docker data line", table.stdout)
			}
			return
		}
		want, err := dockerImagesAndBuildCache(df)
		if err != nil {
			t.Fatalf("docker system df: %v\n%s", err, df)
		}
		if out.DockerData.Size == nil || *out.DockerData.Size != want || out.DockerData.Mark != "reported only" {
			t.Errorf("dockerData = %+v, want size %d marked reported only", out.DockerData, want)
		}
		if !strings.Contains(table.stdout, "reported only") || !strings.Contains(table.stdout, "\nDocker data  ") {
			t.Errorf("table is\n%s\nwant Docker data marked reported only", table.stdout)
		}
	})

	t.Run("an unknown flag is a usage error", func(t *testing.T) {
		if got := b.run(t, dir, "scan", "--jsno"); got.status != 2 || got.stdout != "" {
			t.Errorf("exit status %d, stdout %q, want 2 and nothing", got.status, got.stdout)
		}
	})

}

func TestASecondSpellingOfACandidateDoesNotChangeLastUsed(t *testing.T) {
	b := build(t)
	top := t.TempDir()
	old := time.Now().Add(-60 * 24 * time.Hour)
	recent := time.Now().Add(-time.Hour)
	write := func(rel string, when time.Time) {
		t.Helper()
		p := filepath.Join(top, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatal(err)
		}
	}
	write("app/package.json", old)
	write("app/src/index.js", old)
	write("app/node_modules/keep.js", old)
	write("app/sub/package.json", old)
	write("app/sub/src/index.js", old)
	write("app/sub/node_modules/new.js", recent)
	write("outside/new.js", recent)
	link := func(target, name string) {
		t.Helper()
		err := os.Symlink(filepath.Join(top, filepath.FromSlash(target)), filepath.Join(top, filepath.FromSlash(name)))
		if err != nil && runtime.GOOS == "windows" && os.Getenv("CI") == "" {
			t.Skipf("this account may not make a symlink: %v", err)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	// alias is another spelling of app/sub. jump is a Link, and a Scan does not follow it.
	link("app/sub", "alias")
	link("outside", "app/jump")

	app := filepath.Join(top, "app")
	scan := func(roots ...string) scanJSON {
		t.Helper()
		got := b.run(t, top, append([]string{"scan", "--json"}, roots...)...)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("sweep scan --json %v: exit status %d, stderr %q", roots, got.status, got.stderr)
		}
		return decode(t, got)
	}
	parent := func(out scanJSON) (time.Time, bool) {
		t.Helper()
		var used time.Time
		var stale bool
		var n int
		for _, c := range out.Candidates {
			if sameDir(t, c.Path, filepath.Join(app, "node_modules")) {
				n++
				used, stale = c.LastUsed, c.Stale
			}
		}
		if n != 1 {
			t.Fatalf("app/node_modules is listed %d times in %+v", n, out.Candidates)
		}
		return used, stale
	}
	usedOf := func(path string) time.Time {
		t.Helper()
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		return info.ModTime()
	}

	want := usedOf(filepath.Join(app, "src", "index.js"))
	onlyUsed, onlyStale := parent(scan(app))
	if !onlyStale || !onlyUsed.Equal(want) {
		t.Fatalf("with one Root, Last used = %s, Stale = %v, want %s and Stale", onlyUsed, onlyStale, want)
	}

	both := scan(filepath.Join(top, "alias"), app)
	bothUsed, bothStale := parent(both)
	if !bothStale || !bothUsed.Equal(want) {
		t.Errorf("with both spellings, Last used = %s, Stale = %v, want %s and Stale", bothUsed, bothStale, want)
	}
	nested := 0
	for _, c := range both.Candidates {
		if sameDir(t, c.Path, filepath.Join(app, "sub", "node_modules")) {
			nested++
		}
	}
	if nested != 1 {
		t.Errorf("the nested Candidate is listed %d times, want once", nested)
	}
}

func TestTouchingASourceFileClearsStaleAndACandidateFileDoesNot(t *testing.T) {
	b := build(t)
	dir, _ := b.buildFixture(t)
	root := filepath.Join(dir, "projects")
	project := filepath.Join(dir, "projects", "stale-app")
	candidate := filepath.Join(project, "node_modules")
	scan := func() scanJSON {
		t.Helper()
		got := b.run(t, dir, "scan", "--json", root)
		if got.status != 0 || got.stderr != "" {
			t.Fatalf("sweep scan --json: exit status %d, stderr %q", got.status, got.stderr)
		}
		return decode(t, got)
	}
	one := func(out scanJSON) (last time.Time, stale bool) {
		t.Helper()
		for _, c := range out.Candidates {
			if sameDir(t, c.Path, candidate) {
				return c.LastUsed, c.Stale
			}
		}
		t.Fatal("the Stale Project's Candidate is not listed")
		return time.Time{}, false
	}

	last, stale := one(scan())
	used := newestOutside(t, project, candidate)
	if !stale || !last.Equal(used) {
		t.Fatalf("Last used = %s, Stale = %v, want %s and Stale", last, stale, used)
	}

	inside := filepath.Join(candidate, "fake-old", "index.js")
	if err := os.Chtimes(inside, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	last, stale = one(scan())
	used = newestOutside(t, project, candidate)
	if !stale || !last.Equal(used) {
		t.Errorf("after a file inside the Candidate changed, Last used = %s, Stale = %v, want %s and still Stale", last, stale, used)
	}

	src := filepath.Join(project, "src", "index.js")
	if err := os.Chtimes(src, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	last, stale = one(scan())
	used = newestOutside(t, project, candidate)
	if stale || !last.Equal(used) {
		t.Errorf("after a source file changed, Last used = %s, Stale = %v, want %s and not Stale", last, stale, used)
	}
}

func TestAnUnreadableModificationTimeIsNotStale(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("only an account that is not root can be refused a file by its directory's mode")
	}
	b := build(t)
	top := t.TempDir()
	old := time.Now().Add(-60 * 24 * time.Hour)
	for _, rel := range []string{"app/package.json", "app/src/index.js", "app/node_modules/a/index.js"} {
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
	src := filepath.Join(top, "app", "src")
	hidden := filepath.Join(src, "index.js")
	if err := os.Chmod(src, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(src, 0o755) })
	if _, err := os.ReadDir(src); err != nil {
		t.Skipf("this system will not list a directory it cannot search: %v", err)
	}
	if _, err := os.Lstat(hidden); err == nil {
		t.Fatal("the file's modification time can still be read, so the test shows nothing")
	}

	root := filepath.Join(top, "app")
	got := b.run(t, top, "scan", "--json", root)
	if got.status != 1 {
		t.Fatalf("exit status %d, stderr %q, want 1", got.status, got.stderr)
	}
	out := decode(t, got)
	if len(out.Unreadable) != 1 || filepath.Clean(out.Unreadable[0].Path) != filepath.Clean(hidden) {
		t.Errorf("unreadable = %+v, want %s once", out.Unreadable, hidden)
	}
	if len(out.Candidates) != 1 || out.Candidates[0].Stale {
		t.Errorf("candidates = %+v, want one Candidate that is not Stale", out.Candidates)
	}
}

func TestAnUnreadableDirectoryInAProjectIsListedOnceAndIsNotStale(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("only an account that is not root can be refused a directory by its mode")
	}
	b := build(t)
	top := t.TempDir()
	old := time.Now().Add(-60 * 24 * time.Hour)
	for _, rel := range []string{
		"app/package.json",
		"app/src/index.js",
		"app/node_modules/a/index.js",
		"app/sub/package.json",
		"app/sub/src/index.js",
		"app/sub/node_modules/b/index.js",
		"app/sub/sealed/keep.js",
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
	sealed := filepath.Join(top, "app", "sub", "sealed")
	if err := os.Chmod(sealed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o755) })

	root := filepath.Join(top, "app")
	got := b.run(t, top, "scan", "--json", root)
	if got.status != 1 {
		t.Fatalf("exit status %d, stderr %q, want 1", got.status, got.stderr)
	}
	out := decode(t, got)
	if len(out.Unreadable) != 1 || !sameDir(t, out.Unreadable[0].Path, sealed) {
		t.Errorf("unreadable = %+v, want %s once", out.Unreadable, sealed)
	}
	if len(out.Candidates) != 2 {
		t.Fatalf("candidates = %+v, want the Project and the nested Project", out.Candidates)
	}
	for _, c := range out.Candidates {
		if c.Stale {
			t.Errorf("%s is Stale, and its Project holds a directory that could not be read", c.Path)
		}
	}

	table := b.run(t, top, "scan", root)
	if table.status != 1 {
		t.Fatalf("table exit status %d, want 1", table.status)
	}
	listed := 0
	for _, line := range strings.Split(table.stdout, "\n") {
		if strings.Contains(line, sealed) {
			listed++
		}
	}
	if listed != 1 {
		t.Errorf("the table lists %s on %d lines\n%s", sealed, listed, table.stdout)
	}
}

func TestAScanThatCannotReadADirectoryListsItAndExitsOne(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("only an account that is not root can be refused a directory by its mode")
	}
	b := build(t)
	top := t.TempDir()
	for _, p := range []string{"open/package.json", "open/node_modules/a/index.js", "locked/package.json"} {
		if err := os.MkdirAll(filepath.Join(top, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(top, p), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	locked := filepath.Join(top, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	got := b.run(t, top, "scan", "--json", top)

	if got.status != 1 {
		t.Errorf("exit status %d, want 1", got.status)
	}
	out := decode(t, got)
	if len(out.Unreadable) != 1 || !sameDir(t, out.Unreadable[0].Path, locked) {
		t.Errorf("unreadable = %+v, want %s", out.Unreadable, locked)
	}
	if len(out.Candidates) != 1 || !sameDir(t, out.Candidates[0].Path, filepath.Join(top, "open", "node_modules")) {
		t.Errorf("candidates = %+v, want open/node_modules, which could be read", out.Candidates)
	}
}

func TestAScanThatCannotReadInsideACandidateListsItAndExitsOne(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("only an account that is not root can be refused a directory by its mode")
	}
	b := build(t)
	top := t.TempDir()
	for _, p := range []string{"app/package.json", "app/node_modules/a/index.js", "app/node_modules/sealed/keep.js"} {
		if err := os.MkdirAll(filepath.Join(top, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(top, p), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sealed := filepath.Join(top, "app", "node_modules", "sealed")
	if err := os.Chmod(sealed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o755) })

	got := b.run(t, top, "scan", "--json", top)

	if got.status != 1 {
		t.Errorf("exit status %d, want 1", got.status)
	}
	out := decode(t, got)
	if len(out.Unreadable) != 1 || !sameDir(t, out.Unreadable[0].Path, sealed) {
		t.Errorf("unreadable = %+v, want %s", out.Unreadable, sealed)
	}
	if len(out.Candidates) != 1 || !sameDir(t, out.Candidates[0].Path, filepath.Join(top, "app", "node_modules")) {
		t.Errorf("candidates = %+v, want app/node_modules", out.Candidates)
	}
}

type row struct {
	cells              []string
	size, within       float64
	kind, path, reason string
	stale              bool
}

func readTable(t *testing.T, out string) ([]string, []row) {
	t.Helper()
	lines := strings.Split(out, "\n")
	var roots []string
	for _, l := range lines[1:] {
		if l == "" {
			break
		}
		roots = append(roots, strings.TrimSpace(l))
	}
	header := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "SIZE") })
	if lines[0] != "Roots" || header < 0 {
		t.Fatalf("no Roots and no table in\n%s", out)
	}
	units := map[string]float64{"B": 1, "KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40}
	var rows []row
	for _, l := range lines[header+1:] {
		if strings.HasPrefix(l, "Not read") {
			break
		}
		if l == "" || strings.HasPrefix(l, "SIZE") {
			continue
		}
		cells := regexp.MustCompile(`\s{2,}`).Split(strings.TrimSpace(l), -1)
		number, unit, ok := strings.Cut(cells[0], " ")
		value, err := strconv.ParseFloat(number, 64)
		if !ok || err != nil || units[unit] == 0 {
			continue
		}
		var reason string
		var stale bool
		switch {
		case len(cells) == 6 && cells[4] == "stale":
			stale, reason = true, cells[5]
		case len(cells) == 5:
			reason = cells[4]
		default:
			t.Fatalf("the row %q is not SIZE, KIND, PATH, LAST USED, STALE and REASON", l)
		}
		within := 0.05 * units[unit]
		if unit == "B" {
			within = 0
		}
		rows = append(rows, row{cells, value * units[unit], within, cells[1], cells[2], reason, stale})
	}
	return roots, rows
}

// newestOutside is the newest modification of the regular files in project
// outside the Candidate, which is what Last used must equal.
func newestOutside(t *testing.T, project, candidate string) time.Time {
	t.Helper()
	var newest time.Time
	err := filepath.WalkDir(project, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == project {
			return err
		}
		if p == candidate || strings.HasPrefix(p, candidate+string(filepath.Separator)) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if newest.IsZero() {
		t.Fatalf("%s has no file outside %s", project, candidate)
	}
	return newest
}

func rowFor(t *testing.T, out, path string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, path) {
			return l
		}
	}
	t.Fatalf("no row contains %s\n%s", path, out)
	return ""
}

func TestALinkRootReachedThroughALinkAppliesDotDot(t *testing.T) {
	b := build(t)
	top := t.TempDir()
	for _, p := range []string{
		filepath.Join(top, "deep", "target", "node_modules", "a.js"),
		filepath.Join(top, "target", "node_modules", "b.js"),
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(top, "deep", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(top, "deep", "target", "package.json"),
		filepath.Join(top, "target", "package.json"),
	} {
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join("deep", "dir"), filepath.Join(top, "alias")); err != nil {
		if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
			t.Skipf("this account may not make a symlink: %v", err)
		}
		t.Fatal(err)
	}
	if err := os.Symlink("../target", filepath.Join(top, "deep", "dir", "rootlink")); err != nil {
		t.Fatal(err)
	}

	got := b.run(t, top, "scan", "--json", filepath.Join("alias", "rootlink"))
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit status %d, stderr %q", got.status, got.stderr)
	}
	out := decode(t, got)
	want := filepath.Join(top, "deep", "target", "node_modules")
	decoy := filepath.Join(top, "target", "node_modules")
	if len(out.Candidates) != 1 || !sameDir(t, out.Candidates[0].Path, want) || sameDir(t, out.Candidates[0].Path, decoy) {
		t.Errorf("candidates = %+v, want %s", out.Candidates, want)
	}
	if len(out.Roots) != 1 || !sameDir(t, out.Roots[0], filepath.Join(top, "deep", "target")) || sameDir(t, out.Roots[0], filepath.Join(top, "target")) {
		t.Errorf("roots = %v, want %s", out.Roots, filepath.Join(top, "deep", "target"))
	}
}

func TestDockerSystemDFTotalsAreThePrintedUnits(t *testing.T) {
	out := []byte(strings.Join([]string{
		`{"Type":"Images","Size":"1.5kB"}`,
		`{"Type":"Containers","Size":"9GB"}`,
		`{"Type":"Build Cache","Size":"2MB"}`,
	}, "\n"))
	got, err := dockerImagesAndBuildCache(out)
	if err != nil {
		t.Fatal(err)
	}
	// 1.5kB is 1500 bytes and 2MB is 2000000. Containers are not part of the total.
	if got != 1500+2_000_000 {
		t.Errorf("total = %d, want 2001500", got)
	}
	n, err := dockerHuman("1.086GB")
	if err != nil || n != 1_086_000_000 {
		t.Errorf("1.086GB = %d, %v, want 1086000000", n, err)
	}
}

// dockerImagesAndBuildCache is the Images total plus the Build Cache total
// from `docker system df --format '{{json .}}'`. It is the check against
// sweep, so it does not call sweep.
func dockerImagesAndBuildCache(out []byte) (int64, error) {
	var (
		images, cache       int64
		sawImages, sawCache bool
	)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			Type string
			Size string
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return 0, err
		}
		if row.Type != "Images" && row.Type != "Build Cache" {
			continue
		}
		n, err := dockerHuman(row.Size)
		if err != nil {
			return 0, err
		}
		if row.Type == "Images" {
			if sawImages {
				return 0, fmt.Errorf("duplicate %s", row.Type)
			}
			images, sawImages = n, true
			continue
		}
		if sawCache {
			return 0, fmt.Errorf("duplicate %s", row.Type)
		}
		cache, sawCache = n, true
	}
	if !sawImages || !sawCache {
		return 0, fmt.Errorf("docker system df has no Images or Build Cache total")
	}
	return images + cache, nil
}

func dockerHuman(s string) (int64, error) {
	s = strings.TrimSpace(s)
	i := len(s)
	for i > 0 {
		c := s[i-1]
		if (c >= '0' && c <= '9') || c == '.' {
			break
		}
		i--
	}
	number, unit := strings.TrimSpace(s[:i]), strings.TrimSpace(s[i:])
	value, err := strconv.ParseFloat(number, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("size %q", s)
	}
	mult, ok := map[string]float64{
		"B": 1, "kB": 1e3, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12, "PB": 1e15,
		"KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40,
	}[unit]
	if !ok {
		return 0, fmt.Errorf("unit %q", unit)
	}
	return int64(math.Round(value * mult)), nil
}
