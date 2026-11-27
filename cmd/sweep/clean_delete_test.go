package main_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/system/trash"
)

func TestCleanDeletesThePlanAndLeavesTheTrashEmpty(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.data = filepath.Join(dir, "trash-home")
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	kept := filepath.Join(t.TempDir(), "kept")
	if err := os.MkdirAll(kept, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(kept, "marker.txt")
	if err := os.WriteFile(marker, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkTo(t, kept, at("projects/stale-app/node_modules/escape"))

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
	var sum int64
	inPlan := map[string]bool{}
	for _, c := range key.Candidates {
		if planHolds(t, planned, at(c.Path)) {
			inPlan[c.Path] = true
		}
	}
	for _, c := range planned.Candidates {
		sum += du(t, c.Path)
	}

	got := b.run(t, dir, "clean", "--delete", "--plan", planPath)
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit %d, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	if !strings.HasPrefix(got.stdout, "The removal is permanent.\n") {
		t.Errorf("stdout is\n%s\nwant the permanent line first", got.stdout)
	}
	if strings.Contains(got.stdout, "Moved to trash") {
		t.Errorf("a Delete was reported as trash:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "Reclaimed: "+countPhrase(len(planned.Candidates))+", "+shown(sum)+"\n") {
		t.Errorf("stdout is\n%s\nwant the Plan's Candidates and %d bytes, which du gave", got.stdout, sum)
	}
	for _, c := range planned.Candidates {
		if _, err := os.Lstat(c.Path); !os.IsNotExist(err) {
			t.Errorf("%s is still in place: %v", c.Path, err)
		}
	}
	if body, err := os.ReadFile(marker); err != nil || string(body) != "keep\n" {
		t.Errorf("the Link target is %q, %v", body, err)
	}
	assertTrashEmpty(t, b.data)
	for _, trap := range key.Traps {
		if candidateToo(key, trap.Path) {
			continue
		}
		if _, err := os.Lstat(at(trap.Path)); err != nil {
			t.Errorf("the Trap %s did not survive: %v", trap.Path, err)
		}
	}
	for _, c := range key.Candidates {
		if inPlan[c.Path] {
			continue
		}
		if _, err := os.Lstat(at(c.Path)); err != nil {
			t.Errorf("%s was not in the Plan and is gone: %v", c.Path, err)
		}
	}
	if _, err := os.Lstat(at("projects/stale-app/package.json")); err != nil {
		t.Errorf("the Project's own file is gone: %v", err)
	}
}

func TestCleanSkipsACandidateThatChangedAndDeletesTheRest(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.data = filepath.Join(dir, "trash-home")
	b.config = t.TempDir()
	if runtime.GOOS == "darwin" {
		b.config = b.data
	}
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	projects := at(key.Root)
	writeConfig(t, b.config, "roots = ["+tomlPath(t, projects)+"]\nstale_days = 0\n")
	planPath := filepath.Join(t.TempDir(), "plan.json")
	dry := b.run(t, dir, "clean", "--dry-run", "--plan", planPath)
	if dry.status != 0 || dry.stderr != "" {
		t.Fatalf("dry run exit %d, stderr %q", dry.status, dry.stderr)
	}
	planned := decodePlan(t, planPath)
	changed := at("projects/shifted-app/node_modules")
	if !planHolds(t, planned, changed) {
		t.Fatalf("the Plan does not hold %s", changed)
	}
	added := filepath.Join(changed, "changed.txt")
	if err := os.WriteFile(added, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sum int64
	var deleting int
	for _, c := range planned.Candidates {
		if sameDir(t, c.Path, changed) {
			continue
		}
		sum += du(t, c.Path)
		deleting++
	}

	got := b.run(t, dir, "clean", "--delete", "--plan", planPath)
	if got.status != 1 {
		t.Fatalf("exit %d, want 1, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	if !strings.HasPrefix(got.stdout, "The removal is permanent.\n") {
		t.Errorf("stdout is\n%s\nwant the permanent line first", got.stdout)
	}
	if strings.Contains(got.stdout, "Moved to trash") {
		t.Errorf("a Delete was reported as trash:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "Fingerprint rule") || !strings.Contains(got.stdout, changed) {
		t.Errorf("stdout is\n%s\nwant a Skip of %s under the Fingerprint rule", got.stdout, changed)
	}
	if !strings.Contains(got.stdout, "Reclaimed: "+countPhrase(deleting)+", "+shown(sum)+"\n") {
		t.Errorf("stdout is\n%s\nwant %s deleted, %d bytes from du", got.stdout, countPhrase(deleting), sum)
	}
	body, err := os.ReadFile(added)
	if err != nil || string(body) != "changed\n" {
		t.Errorf("the changed Candidate: %q, %v", body, err)
	}
	for _, c := range planned.Candidates {
		_, statErr := os.Lstat(c.Path)
		if sameDir(t, c.Path, changed) {
			if statErr != nil {
				t.Errorf("the Skip is gone: %v", statErr)
			}
			continue
		}
		if !os.IsNotExist(statErr) {
			t.Errorf("%s is still in place: %v", c.Path, statErr)
		}
	}
	assertTrashEmpty(t, b.data)
	for _, trap := range key.Traps {
		if candidateToo(key, trap.Path) {
			continue
		}
		if _, err := os.Lstat(at(trap.Path)); err != nil {
			t.Errorf("the Trap %s did not survive: %v", trap.Path, err)
		}
	}
}

func TestCleanDeleteSkipsWhenADirectoryAboveBecomesALink(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.data = filepath.Join(dir, "trash-home")
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	root := at(key.Root)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	dry := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, root)
	if dry.status != 0 || dry.stderr != "" {
		t.Fatalf("dry run exit %d, stderr %q", dry.status, dry.stderr)
	}
	app := at("projects/stale-app")
	held := app + ".held"
	if err := trash.Place(app, held); err != nil {
		t.Fatal(err)
	}
	linkTo(t, at("outside/stash-app"), app)
	precious := at("outside/stash-app/node_modules/fake-theta/index.js")
	heldNote := filepath.Join(held, "node_modules", "fake-old", "note.txt")
	beforePrecious, err := os.ReadFile(precious)
	if err != nil {
		t.Fatal(err)
	}
	beforeHeld, err := os.ReadFile(heldNote)
	if err != nil {
		t.Fatal(err)
	}

	got := b.run(t, dir, "clean", "--delete", "--plan", planPath)

	afterPrecious, err := os.ReadFile(precious)
	if err != nil || string(afterPrecious) != string(beforePrecious) {
		t.Errorf("the directory the Link leads to changed: %q, %v", afterPrecious, err)
	}
	afterHeld, err := os.ReadFile(heldNote)
	if err != nil || string(afterHeld) != string(beforeHeld) {
		t.Errorf("the Candidate that was renamed aside changed: %q, %v", afterHeld, err)
	}
	if got.status != 1 {
		t.Fatalf("exit %d, want 1, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	if !strings.HasPrefix(got.stdout, "The removal is permanent.\n") {
		t.Errorf("stdout is\n%s\nwant the permanent line first", got.stdout)
	}
	candidate := filepath.Join(app, "node_modules")
	if !strings.Contains(got.stdout, "Root rule") || !strings.Contains(got.stdout, candidate) {
		t.Errorf("stdout is\n%s\nwant a Skip of %s under the Root rule", got.stdout, candidate)
	}
	if strings.Contains(got.stdout, "Reclaimed: 1") {
		t.Errorf("a Candidate was deleted:\n%s", got.stdout)
	}
	assertTrashEmpty(t, b.data)
}

func TestDeleteOnADryRunRemovesNothing(t *testing.T) {
	b := build(t)
	dir, _ := b.buildFixture(t)
	before := listing(t, dir)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	got := b.run(t, dir, "clean", "--delete", "--dry-run", "--plan", planPath, "projects")
	if got.status != 2 {
		t.Fatalf("exit status %d, want 2, stderr %q", got.status, got.stderr)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want nothing", got.stdout)
	}
	if !strings.Contains(got.stderr, "--delete") || !strings.Contains(got.stderr, "--dry-run") {
		t.Errorf("stderr = %q, want the clean usage", got.stderr)
	}
	if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
		t.Errorf("wrote a Plan: %v", err)
	}
	for _, d := range differences(before, listing(t, dir)) {
		t.Errorf("the Fixture changed: %s", d)
	}

	alone := b.run(t, dir, "clean", "--delete")
	if alone.status != 2 || alone.stdout != "" {
		t.Fatalf("--delete alone: exit %d, stdout %q, stderr %q", alone.status, alone.stdout, alone.stderr)
	}
	for _, d := range differences(before, listing(t, dir)) {
		t.Errorf("the Fixture changed: %s", d)
	}
}

func assertTrashEmpty(t *testing.T, dataHome string) {
	t.Helper()
	dirs := []string{filepath.Join(dataHome, "Trash", "files"), filepath.Join(dataHome, "Trash", "info")}
	if runtime.GOOS == "darwin" {
		dirs = []string{filepath.Join(dataHome, ".Trash"), filepath.Join(dataHome, ".Trash", ".info")}
	}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) > 0 {
			names := make([]string, len(entries))
			for i, e := range entries {
				names[i] = e.Name()
			}
			t.Errorf("the trash %s holds %v", d, names)
		}
	}
}
