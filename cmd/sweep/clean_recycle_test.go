//go:build windows

package main_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/system/trash"
)

func TestCleanUsesTheRecycleBinWhenTheTrashDirectoryIsUnset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	b := build(t)
	b.omitData = true
	dir, key := b.buildFixture(t)
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	root := at(key.Root)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	dry := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, root)
	if dry.status != 0 || dry.stderr != "" {
		t.Fatalf("dry run exit %d, stderr %q", dry.status, dry.stderr)
	}
	planned := decodePlan(t, planPath)
	if len(planned.Candidates) == 0 {
		t.Fatal("the Plan is empty")
	}

	sizes := map[string]int64{}
	notes := map[string][]byte{}
	for _, c := range planned.Candidates {
		sizes[c.Path] = du(t, c.Path)
		if data, err := os.ReadFile(filepath.Join(c.Path, "fake-old", "note.txt")); err == nil {
			notes[c.Path] = data
		}
		path := c.Path
		t.Cleanup(func() {
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				return
			}
			mover, err := trash.Open()
			if err != nil {
				return
			}
			_ = mover.Restore(root, path)
		})
	}
	trapSize := map[string]int64{}
	for _, trap := range key.Traps {
		if planHolds(t, planned, at(trap.Path)) {
			continue
		}
		trapSize[trap.Path] = du(t, at(trap.Path))
	}

	before := listing(t, dir)
	got := b.run(t, dir, "clean", "--plan", planPath)
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit %d, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	after := listing(t, dir)
	for _, c := range planned.Candidates {
		if _, ok := before[c.Path]; !ok {
			t.Errorf("%s was not listed before the Clean", c.Path)
		}
		if _, ok := after[c.Path]; ok {
			t.Errorf("%s is still listed", c.Path)
		}
	}
	for _, trap := range key.Traps {
		if planHolds(t, planned, at(trap.Path)) {
			continue
		}
		p := at(trap.Path)
		was, ok := before[p]
		if !ok {
			t.Errorf("the Trap %s was not listed", trap.Path)
			continue
		}
		now, ok := after[p]
		if !ok || now != was {
			t.Errorf("the Trap %s changed in the listing", trap.Path)
		}
		if du(t, p) != trapSize[trap.Path] {
			t.Errorf("the Trap %s is %d bytes, du said %d", trap.Path, du(t, p), trapSize[trap.Path])
		}
	}

	mover, err := trash.Open()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range planned.Candidates {
		if err := mover.Restore(root, c.Path); err != nil {
			t.Fatalf("restore %s: %v", c.Path, err)
		}
		if du(t, c.Path) != sizes[c.Path] {
			t.Errorf("%s is %d bytes, du said %d before the Clean", c.Path, du(t, c.Path), sizes[c.Path])
		}
		if data, ok := notes[c.Path]; ok {
			back, err := os.ReadFile(filepath.Join(c.Path, "fake-old", "note.txt"))
			if err != nil || string(back) != string(data) {
				t.Errorf("restored note is %q, %v", back, err)
			}
		}
	}
}
