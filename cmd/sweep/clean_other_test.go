//go:build !linux

package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanRefusesWhereThereIsNoTrash(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.data = filepath.Join(dir, "trash-home")
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	planPath := filepath.Join(t.TempDir(), "plan.json")
	dry := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, at(key.Root))
	if dry.status != 0 || dry.stderr != "" {
		t.Fatalf("dry run exit %d, stderr %q", dry.status, dry.stderr)
	}
	before := listing(t, dir)
	got := b.run(t, dir, "clean", "--plan", planPath)
	if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, "no trash") {
		t.Fatalf("exit %d, stdout %q, stderr %q, want a refusal", got.status, got.stdout, got.stderr)
	}
	for _, d := range differences(before, listing(t, dir)) {
		t.Errorf("a refused Clean changed the Fixture: %s", d)
	}
}
