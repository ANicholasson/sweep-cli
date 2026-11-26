package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCleanMovesThePlanAndNothingElse(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("only Linux moves a Candidate to the trash; other systems refuse a Clean")
	}
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
	planned := decodePlan(t, planPath)
	if len(planned.Candidates) == 0 {
		t.Fatal("the Plan is empty")
	}
	var sum int64
	saved := map[string][]byte{}
	inPlan := map[string]bool{}
	for _, c := range key.Candidates {
		if planHolds(t, planned, at(c.Path)) {
			inPlan[c.Path] = true
		}
	}
	for _, c := range planned.Candidates {
		sum += du(t, c.Path)
		note := filepath.Join(c.Path, "fake-old", "note.txt")
		if data, err := os.ReadFile(note); err == nil {
			saved[c.Path] = data
		}
	}
	if len(saved) == 0 {
		t.Fatal("no planned Candidate held fake-old/note.txt, so a restore would show nothing")
	}

	got := b.run(t, dir, "clean", "--plan", planPath)
	if got.status != 0 || got.stderr != "" {
		t.Fatalf("exit %d, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	if strings.Contains(strings.ToLower(got.stdout), "reclaimed") {
		t.Errorf("trashed space is called reclaimed:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "Moved to trash: "+countPhrase(len(planned.Candidates))+", "+shown(sum)+"\n") {
		t.Errorf("stdout is\n%s\nwant the Plan's Candidates and %d bytes, which du gave", got.stdout, sum)
	}
	for _, c := range planned.Candidates {
		if _, err := os.Lstat(c.Path); !os.IsNotExist(err) {
			t.Errorf("%s is still in place: %v", c.Path, err)
		}
		landed := trashedCopy(t, b.data, c.Path)
		if data, ok := saved[c.Path]; ok {
			gotData, err := os.ReadFile(filepath.Join(landed, "fake-old", "note.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if string(gotData) != string(data) {
				t.Errorf("the trashed note is %q, want %q", gotData, data)
			}
			if err := os.Rename(landed, c.Path); err != nil {
				t.Fatal(err)
			}
			again, err := os.ReadFile(filepath.Join(c.Path, "fake-old", "note.txt"))
			if err != nil || string(again) != string(data) {
				t.Errorf("restoring %s gave %q, %v", c.Path, again, err)
			}
			if err := os.Rename(c.Path, landed); err != nil {
				t.Fatal(err)
			}
		}
	}
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

func TestCleanSkipsACandidateThatChangedAndTrashesTheRest(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("only Linux moves a Candidate to the trash; other systems refuse a Clean")
	}
	b := build(t)
	dir, key := b.buildFixture(t)
	b.data = filepath.Join(dir, "trash-home")
	b.config = t.TempDir()
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
	var moving int
	for _, c := range planned.Candidates {
		if sameDir(t, c.Path, changed) {
			continue
		}
		sum += du(t, c.Path)
		moving++
	}

	got := b.run(t, dir, "clean", "--plan", planPath)
	if got.status != 1 {
		t.Fatalf("exit %d, want 1, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	if strings.Contains(strings.ToLower(got.stdout), "reclaimed") {
		t.Errorf("trashed space is called reclaimed:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "Fingerprint rule") || !strings.Contains(got.stdout, changed) {
		t.Errorf("stdout is\n%s\nwant a Skip of %s under the Fingerprint rule", got.stdout, changed)
	}
	if !strings.Contains(got.stdout, "Moved to trash: "+countPhrase(moving)+", "+shown(sum)+"\n") {
		t.Errorf("stdout is\n%s\nwant %s moved, %d bytes from du", got.stdout, countPhrase(moving), sum)
	}
	body, err := os.ReadFile(added)
	if err != nil || string(body) != "changed\n" {
		t.Errorf("the changed Candidate: %q, %v", body, err)
	}
	for _, c := range planned.Candidates {
		_, err := os.Lstat(c.Path)
		if sameDir(t, c.Path, changed) {
			if err != nil {
				t.Errorf("the Skip is gone: %v", err)
			}
			continue
		}
		if !os.IsNotExist(err) {
			t.Errorf("%s is still in place: %v", c.Path, err)
		}
		if _, err := os.Lstat(trashedCopy(t, b.data, c.Path)); err != nil {
			t.Errorf("%s is not in the trash: %v", c.Path, err)
		}
	}
	for _, trap := range key.Traps {
		if _, err := os.Lstat(at(trap.Path)); err != nil {
			t.Errorf("the Trap %s did not survive: %v", trap.Path, err)
		}
	}
}

func TestCleanSkipsWhenADirectoryAboveBecomesALink(t *testing.T) {
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
	if err := os.Rename(app, held); err != nil {
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

	got := b.run(t, dir, "clean", "--plan", planPath)

	afterPrecious, err := os.ReadFile(precious)
	if err != nil || string(afterPrecious) != string(beforePrecious) {
		t.Errorf("the directory the Link leads to changed: %q, %v", afterPrecious, err)
	}
	afterHeld, err := os.ReadFile(heldNote)
	if err != nil || string(afterHeld) != string(beforeHeld) {
		t.Errorf("the Candidate that was renamed aside changed: %q, %v", afterHeld, err)
	}
	if runtime.GOOS != "linux" {
		if got.status != 2 || got.stdout != "" || !strings.Contains(got.stderr, "no trash") {
			t.Fatalf("exit %d, stdout %q, stderr %q, want a refusal and nothing removed", got.status, got.stdout, got.stderr)
		}
		return
	}
	if got.status != 1 {
		t.Fatalf("exit %d, want 1, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	candidate := filepath.Join(app, "node_modules")
	if !strings.Contains(got.stdout, "Root rule") || !strings.Contains(got.stdout, candidate) {
		t.Errorf("stdout is\n%s\nwant a Skip of %s under the Root rule", got.stdout, candidate)
	}
	if strings.Contains(got.stdout, "Moved to trash: 1") {
		t.Errorf("a Candidate was moved:\n%s", got.stdout)
	}
}

func TestCleanRefusesAMalformedPlan(t *testing.T) {
	b := build(t)
	dir, _ := b.buildFixture(t)
	b.data = filepath.Join(dir, "trash-home")
	before := listing(t, dir)
	for _, body := range []string{
		"{}\n",
		"{\"version\":2,\"roots\":[],\"candidates\":[]}\n",
		"{\"version\":1,\"roots\":[],\"candidates\":[],\"extra\":true}\n",
		"not a plan\n",
	} {
		planPath := filepath.Join(t.TempDir(), "plan.json")
		if err := os.WriteFile(planPath, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		got := b.run(t, dir, "clean", "--plan", planPath)
		if got.status != 2 || got.stdout != "" {
			t.Fatalf("plan %s: exit %d, stdout %q, stderr %q, want a refusal", body, got.status, got.stdout, got.stderr)
		}
		if !strings.Contains(got.stderr, "malformed") && !strings.Contains(got.stderr, "version") {
			t.Errorf("stderr = %q, want the Plan refused", got.stderr)
		}
	}
	for _, d := range differences(before, listing(t, dir)) {
		t.Errorf("the Fixture changed: %s", d)
	}
}

func TestCleanLeavesACandidateTheTrashRefuses(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("only Linux tries the trash; other systems refuse a Clean first")
	}
	b := build(t)
	dir, key := b.buildFixture(t)
	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	b.data = blocker
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	planPath := filepath.Join(t.TempDir(), "plan.json")
	dry := b.run(t, dir, "clean", "--dry-run", "--plan", planPath, at(key.Root))
	if dry.status != 0 || dry.stderr != "" {
		t.Fatalf("dry run exit %d, stderr %q", dry.status, dry.stderr)
	}
	planned := decodePlan(t, planPath)
	got := b.run(t, dir, "clean", "--plan", planPath)
	if got.status != 1 {
		t.Fatalf("exit %d, want 1, stderr %q\n%s", got.status, got.stderr, got.stdout)
	}
	if strings.Contains(strings.ToLower(got.stdout), "reclaimed") || !strings.Contains(got.stdout, "Failed\n") {
		t.Errorf("stdout is\n%s\nwant a failure and no reclaimed space", got.stdout)
	}
	for _, c := range planned.Candidates {
		if _, err := os.Lstat(c.Path); err != nil {
			t.Errorf("%s is gone after the trash refused it: %v", c.Path, err)
		}
	}
	for _, trap := range key.Traps {
		if _, err := os.Lstat(at(trap.Path)); err != nil {
			t.Errorf("the Trap %s did not survive: %v", trap.Path, err)
		}
	}
}

func trashedCopy(t *testing.T, dataHome, original string) string {
	t.Helper()
	info := filepath.Join(dataHome, "Trash", "info")
	entries, err := os.ReadDir(info)
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(info, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "Path="+abs+"\n") {
			return filepath.Join(dataHome, "Trash", "files", strings.TrimSuffix(e.Name(), ".trashinfo"))
		}
	}
	t.Fatalf("%s is not in the trash", original)
	return ""
}

func linkTo(t *testing.T, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err == nil {
		return
	}
	if runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	out, jerr := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if jerr != nil {
		if os.Getenv("CI") == "" {
			t.Skipf("this account may not make a Link: %v", err)
		}
		t.Fatalf("mklink /J: %v: %s", jerr, out)
	}
}
