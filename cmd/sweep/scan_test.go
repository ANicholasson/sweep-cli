package main_test

import (
	"encoding/json"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type scanJSON struct {
	Version    int      `json:"version"`
	Roots      []string `json:"roots"`
	Candidates []struct {
		Path      string `json:"path"`
		Size      int64  `json:"size"`
		Kind      string `json:"kind"`
		Ecosystem string `json:"ecosystem"`
		Reason    string `json:"reason"`
	} `json:"candidates"`
	Unreadable []struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	} `json:"unreadable"`
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

	t.Run("an unknown flag is a usage error", func(t *testing.T) {
		if got := b.run(t, dir, "scan", "--jsno"); got.status != 2 || got.stdout != "" {
			t.Errorf("exit status %d, stdout %q, want 2 and nothing", got.status, got.stdout)
		}
	})

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

type row struct {
	cells              []string
	size, within       float64
	kind, path, reason string
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
		if l == "" {
			break
		}
		cells := regexp.MustCompile(`\s{2,}`).Split(strings.TrimSpace(l), -1)
		number, unit, _ := strings.Cut(cells[0], " ")
		value, err := strconv.ParseFloat(number, 64)
		if len(cells) != 4 || err != nil || units[unit] == 0 {
			t.Fatalf("the row %q is not SIZE, KIND, PATH and REASON", l)
		}
		within := 0.05 * units[unit]
		if unit == "B" {
			within = 0
		}
		rows = append(rows, row{cells, value * units[unit], within, cells[1], cells[2], cells[3]})
	}
	return roots, rows
}
