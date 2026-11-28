package main_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/system/trash"
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
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	stale, recent := splitByAge(t, key, at)
	if len(stale) == 0 || len(recent) == 0 {
		t.Fatalf("stale = %d, recent = %d, want both, from the files' modification times", len(stale), len(recent))
	}
	cache := answerCache(t, key, at)
	want := append(append([]string{}, stale...), cache)
	var sum int64
	for _, path := range want {
		sum += du(t, path)
	}
	cliPlan := filepath.Join(t.TempDir(), "plan.json")
	dry := b.runWith(t, dir, env, "clean", "--dry-run", "--plan", cliPlan, "--pick", cache)
	if dry.status != 0 || dry.stderr != "" {
		t.Fatalf("dry run exit %d, stderr %q\n%s", dry.status, dry.stderr, dry.stdout)
	}
	planned := decodePlan(t, cliPlan)
	if !sameSet(t, planPaths(t, planned), want) {
		t.Fatalf("command line Plan paths = %v, want the Stale Candidates and the ticked cache %v", planPaths(t, planned), want)
	}
	inWant := map[string]bool{}
	for _, c := range key.Candidates {
		if listedIn(t, at(c.Path), want) {
			inWant[c.Path] = true
		}
	}
	screenPlan := writtenPlan(b)

	session := startTerminal(t, b.sweep, dir, env)
	t.Cleanup(session.close)
	session.waitFor(t, "enter review")
	session.send(t, []byte{'\r'})
	review := session.waitFor(t, "will be moved to trash")
	for _, path := range stale {
		if !strings.Contains(review, path) {
			t.Errorf("the review leaves out the Stale Candidate %s\n%s", path, review)
		}
	}
	for _, path := range append(append([]string{}, recent...), cache) {
		if strings.Contains(review, path) {
			t.Errorf("the review includes %s, which is not in the default Pick\n%s", path, review)
		}
	}
	if !strings.Contains(review, "j/k move    y confirm    n back") {
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
	for _, path := range want {
		if !strings.Contains(again, path) {
			t.Errorf("the review leaves out %s\n%s", path, again)
		}
	}
	for _, path := range recent {
		if strings.Contains(again, path) {
			t.Errorf("the review includes the recent Candidate %s\n%s", path, again)
		}
	}
	session.send(t, []byte("y"))
	outcome := session.waitFor(t, "Moved to trash")
	if strings.Contains(strings.ToLower(outcome), "reclaimed") {
		t.Errorf("trashed space is called reclaimed:\n%s", outcome)
	}
	wantSummary := "Moved to trash: " + countPhrase(len(want)) + ", " + shown(sum)
	if !strings.Contains(outcome, wantSummary) {
		t.Errorf("outcome is\n%s\nwant %s, which du gave", outcome, wantSummary)
	}
	for _, path := range want {
		if !strings.Contains(outcome, path) {
			t.Errorf("the outcome leaves out %s\n%s", path, outcome)
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
	cli, err := os.ReadFile(cliPlan)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, cli) {
		t.Fatalf("the screen Plan is\n%s\nthe command line Plan is\n%s", got, cli)
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
		if inWant[c.Path] {
			continue
		}
		path := at(c.Path)
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s was not in the Plan and is gone: %v", path, err)
		}
	}
}

func TestTheScreenSkipsACandidateChangedAfterTheReview(t *testing.T) {
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
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	stale, _ := splitByAge(t, key, at)
	if len(stale) == 0 {
		t.Fatal("no Stale Candidate, from the files' modification times")
	}
	changed := stale[0]

	session := startTerminal(t, b.sweep, dir, env)
	t.Cleanup(session.close)
	session.waitFor(t, "enter review")
	session.send(t, []byte{'\r'})
	review := session.waitFor(t, "will be moved to trash")
	if !strings.Contains(review, changed) {
		t.Fatalf("the review leaves out %s\n%s", changed, review)
	}
	added := filepath.Join(changed, "added.bin")
	payload := bytes.Repeat([]byte("x"), 64)
	if err := os.WriteFile(added, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	session.send(t, []byte("y"))
	outcome := session.waitFor(t, "Fingerprint rule")
	if !strings.Contains(outcome, changed) {
		t.Errorf("the Skip leaves out %s\n%s", changed, outcome)
	}
	session.send(t, []byte("q"))
	if code := session.finish(t); code != 1 {
		t.Fatalf("exit %d, want 1\n%s\n%s", code, session.frame(), session.errText())
	}
	body, err := os.ReadFile(added)
	if err != nil || !bytes.Equal(body, payload) {
		t.Fatalf("the changed file is %q, %v", body, err)
	}
	if _, err := os.Lstat(changed); err != nil {
		t.Errorf("the changed Candidate is gone: %v", err)
	}
}

func TestTheReviewShowsTheSizeRecordedAtEnter(t *testing.T) {
	b := build(t)
	dir, key := b.buildFixture(t)
	b.home = filepath.Join(dir, "home")
	b.cache = t.TempDir()
	if runtime.GOOS != "darwin" {
		b.data = filepath.Join(dir, "trash-home")
	}
	env := b.commandEnv(t)
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	stale, _ := splitByAge(t, key, at)
	if len(stale) == 0 {
		t.Fatal("no Stale Candidate, from the files' modification times")
	}
	changed := stale[0]
	session := startTerminal(t, b.sweep, dir, env)
	t.Cleanup(session.close)
	session.waitFor(t, "enter review")
	added := filepath.Join(changed, "added.bin")
	if err := os.WriteFile(added, bytes.Repeat([]byte("x"), 256*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, path := range stale {
		sum += du(t, path)
	}
	grew := du(t, changed)
	session.send(t, []byte{'\r'})
	review := session.waitFor(t, "will be moved to trash")
	if !reviewLine(review, changed, shown(grew)) {
		t.Fatalf("the review does not show du's %s (%d) for %s\n%s", shown(grew), grew, changed, review)
	}
	want := countPhrase(len(stale)) + ", " + shown(sum) + " will be moved to trash"
	if !strings.Contains(review, want) {
		t.Fatalf("the review total is not %s, which du gave\n%s", want, review)
	}
	session.send(t, []byte("q"))
	if code := session.finish(t); code != 0 {
		t.Fatalf("exit %d, want 0\n%s\n%s", code, session.frame(), session.errText())
	}
	if _, err := os.Lstat(added); err != nil {
		t.Errorf("the added file is gone: %v", err)
	}
}

func reviewLine(frame, path, size string) bool {
	for _, line := range strings.Split(frame, "\n") {
		if strings.Contains(line, path) && strings.Contains(line, size) {
			return true
		}
	}
	return false
}

func TestTheScreenReturnsToTheListWhenThePlanCannotBeRecorded(t *testing.T) {
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
	at := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	stale, _ := splitByAge(t, key, at)
	if len(stale) == 0 {
		t.Fatal("no Stale Candidate, from the files' modification times")
	}
	changed := stale[0]
	note, err := os.ReadFile(filepath.Join(changed, "fake-old", "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	session := startTerminal(t, b.sweep, dir, env)
	t.Cleanup(session.close)
	session.waitFor(t, "enter review")
	aside := changed + ".aside"
	if err := trash.Place(changed, aside); err != nil {
		t.Fatal(err)
	}
	session.send(t, []byte{'\r'})
	back := session.waitFor(t, "sweep:")
	if !strings.Contains(back, "enter review") || !strings.Contains(back, "[x]") {
		t.Fatalf("the list did not return with the tick:\n%s", back)
	}
	if strings.Contains(back, "q quit") && !strings.Contains(back, "enter review") {
		t.Fatalf("the error opened the outcome:\n%s", back)
	}
	session.send(t, []byte("n"))
	if again := session.waitFor(t, "sweep:"); !strings.Contains(again, "enter review") {
		t.Fatalf("n left the list:\n%s", again)
	}
	session.send(t, []byte("q"))
	if code := session.finish(t); code != 0 {
		t.Fatalf("exit %d, want 0\n%s\n%s", code, session.frame(), session.errText())
	}
	if _, err := os.Lstat(writtenPlan(b)); !os.IsNotExist(err) {
		t.Fatalf("a Plan was written: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(aside, "fake-old", "note.txt"))
	if err != nil || !bytes.Equal(got, note) {
		t.Fatalf("the Candidate moved aside is %q, %v", got, err)
	}
}

func answerCache(t *testing.T, key answerKey, at func(string) string) string {
	t.Helper()
	var rel string
	for _, c := range key.Candidates {
		if strings.HasPrefix(c.Path, key.Root+"/") {
			continue
		}
		if rel != "" {
			t.Fatalf("Candidates outside the Root: %s and %s", rel, c.Path)
		}
		rel = c.Path
	}
	if rel == "" {
		t.Fatal("the Answer key has no Shared cache")
	}
	return at(rel)
}

func listedIn(t *testing.T, path string, want []string) bool {
	t.Helper()
	for _, w := range want {
		if sameDir(t, path, w) {
			return true
		}
	}
	return false
}

func writtenPlan(b binaries) string {
	if runtime.GOOS == "darwin" {
		return planFile(b.home)
	}
	return planFile(b.cache)
}
