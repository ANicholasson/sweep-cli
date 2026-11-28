package main_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTheScreenReviewsAndCleansTheSamePlan(t *testing.T) {
	if !movesToTrash() {
		t.Skip("this operating system has no trash yet, so a Clean removes nothing")
	}
	b := build(t)
	dir, key := b.buildFixture(t)
	b.home = filepath.Join(dir, "home")
	b.cache = t.TempDir()
	if runtime.GOOS != "darwin" {
		b.data = filepath.Join(dir, "trash-home")
	}
	env := b.commandEnv(t)
	cache := sharedCache(b.home)
	cliPlan := filepath.Join(t.TempDir(), "plan.json")
	dry := b.runWith(t, dir, env, "clean", "--dry-run", "--plan", cliPlan, "--pick", cache)
	if dry.status != 0 || dry.stderr != "" {
		t.Fatalf("dry run exit %d, stderr %q\n%s", dry.status, dry.stderr, dry.stdout)
	}
	planned := decodePlan(t, cliPlan)
	if len(planned.Candidates) < 2 || !planHolds(t, planned, cache) {
		t.Fatalf("Plan paths = %v, want the default Pick and %s", planPaths(t, planned), cache)
	}
	var sum int64
	for _, c := range planned.Candidates {
		sum += du(t, c.Path)
	}
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	inPlan := map[string]bool{}
	for _, c := range key.Candidates {
		if planHolds(t, planned, at(c.Path)) {
			inPlan[c.Path] = true
		}
	}
	screenPlan := writtenPlan(b)

	session := startTerminal(t, b.sweep, dir, env)
	t.Cleanup(session.close)
	session.waitFor(t, "enter review")
	session.send(t, []byte{'\r'})
	review := session.waitFor(t, "will be moved to trash")
	if strings.Contains(review, cache) {
		t.Fatalf("the review includes the unticked Shared cache:\n%s", review)
	}
	for _, c := range planned.Candidates {
		if sameDir(t, c.Path, cache) {
			continue
		}
		if !strings.Contains(review, c.Path) {
			t.Errorf("the review leaves out %s\n%s", c.Path, review)
		}
	}
	if !strings.Contains(review, "y confirm    n back") {
		t.Fatalf("review =\n%s\nwant the confirm keys", review)
	}
	session.send(t, []byte("n"))
	session.waitFor(t, "enter review")
	if _, err := os.Lstat(screenPlan); !os.IsNotExist(err) {
		t.Fatalf("cancel wrote %s: %v", screenPlan, err)
	}

	session.highlight(t, "_cacache")
	session.send(t, []byte("t"))
	deadline := session.waitFor(t, "> [x]")
	if !strings.Contains(deadline, "_cacache") {
		t.Fatalf("the tick missed the Shared cache:\n%s", deadline)
	}
	session.send(t, []byte{'\r'})
	again := session.waitForAll(t, "will be moved to trash", cache)
	for _, c := range planned.Candidates {
		if !strings.Contains(again, c.Path) {
			t.Errorf("the review leaves out %s\n%s", c.Path, again)
		}
	}
	session.send(t, []byte("y"))
	outcome := session.waitFor(t, "Moved to trash")
	if strings.Contains(strings.ToLower(outcome), "reclaimed") {
		t.Errorf("trashed space is called reclaimed:\n%s", outcome)
	}
	wantSummary := "Moved to trash: " + countPhrase(len(planned.Candidates)) + ", " + shown(sum)
	if !strings.Contains(outcome, wantSummary) {
		t.Errorf("outcome is\n%s\nwant %s, which du gave", outcome, wantSummary)
	}
	for _, c := range planned.Candidates {
		if !strings.Contains(outcome, c.Path) {
			t.Errorf("the outcome leaves out %s\n%s", c.Path, outcome)
		}
	}
	session.send(t, []byte("q"))
	if code := session.finish(t); code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, session.frame(), session.errText())
	}

	got, err := os.ReadFile(screenPlan)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(cliPlan)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("the screen Plan is\n%s\nthe command line Plan is\n%s", got, want)
	}

	trashAt := b.data
	if runtime.GOOS == "darwin" {
		trashAt = b.home
	}
	for _, c := range planned.Candidates {
		if _, err := os.Lstat(c.Path); !os.IsNotExist(err) {
			t.Errorf("%s is still in place: %v", c.Path, err)
		}
		trashedCopy(t, trashAt, c.Path)
	}
	for _, trap := range key.Traps {
		if _, err := os.Lstat(at(trap.Path)); err != nil {
			t.Errorf("the Trap %s did not survive: %v", trap.Path, err)
		}
		if strings.Contains(outcome, at(trap.Path)) {
			t.Errorf("the outcome names the Trap %s\n%s", trap.Path, outcome)
		}
	}
	for _, c := range key.Candidates {
		if inPlan[c.Path] {
			continue
		}
		path := at(c.Path)
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s was not in the Plan and is gone: %v", path, err)
		}
	}
}

func sharedCache(home string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Local", "npm-cache", "_cacache")
	}
	return filepath.Join(home, ".npm", "_cacache")
}

func writtenPlan(b binaries) string {
	if runtime.GOOS == "darwin" {
		return planFile(b.home)
	}
	return planFile(b.cache)
}
