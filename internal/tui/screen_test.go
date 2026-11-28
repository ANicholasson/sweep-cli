package tui_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/tui"
)

func TestTheScreenStartsFromTheDefaultPick(t *testing.T) {
	project := candidate("/work/app", "/work/app/node_modules", domain.Dependencies, "npm install puts it back from package.json and the lockfile.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), true, false)
	recent := candidate("/work/app", "/work/app/dist", domain.BuildOutput, "The compiler writes it again.", 2048, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), false, false)
	cache := candidate("", "/home/ada/.npm/_cacache", domain.Cache, "npm downloads again whatever a later install needs.", 4096, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), true, true)
	screen := tui.New(scan.Result{
		Candidates: []domain.Candidate{project, recent, cache},
		DockerData: domain.Reported(1 << 30),
	})

	text := screen.Text()
	if strings.Index(text, "/work/app\n") > strings.Index(text, "Shared caches\n") || !strings.Contains(text, "Shared caches\n") {
		t.Fatalf("Candidates are not grouped by Project with Shared caches apart:\n%s", text)
	}
	assertRow(t, text, "node_modules", "> [x]", "1.0 KiB", "Dependencies", "2026-09-01 00:00:00Z")
	assertRow(t, text, "dist", "  [ ]", "2.0 KiB", "Build output", "2026-11-20 00:00:00Z")
	assertRow(t, text, cache.Path, "  [ ]", "4.0 KiB", "Cache", "2026-08-01 00:00:00Z")
	if reason := between(t, text, "Reason\n", "\n\n"); reason != string(project.Rule.Reason) {
		t.Errorf("highlighted Reason = %q, want %q", reason, project.Rule.Reason)
	}
	if strings.Contains(text, string(recent.Rule.Reason)) || strings.Contains(text, string(cache.Rule.Reason)) {
		t.Errorf("a Reason other than the highlighted Candidate's is visible:\n%s", text)
	}
	assertTotal(t, text, "1 Candidate, 1.0 KiB picked")
	if !strings.Contains(text, "j/k move    t tick    u untick    q quit\n") {
		t.Errorf("the keys are missing:\n%s", text)
	}
	docker := lineContaining(t, text, "Docker data")
	if strings.Contains(docker, "[") || !strings.Contains(docker, "1.0 GiB") || !strings.Contains(docker, "reported only") {
		t.Errorf("docker line = %q, want the size, reported only, and no tick", docker)
	}

	if screen.Press([]byte("k")) || screen.Text() != text {
		t.Fatalf("k at the top moved:\n%s", screen.Text())
	}
	if screen.Press([]byte("t")) || screen.Text() != text {
		t.Fatalf("ticking the highlighted Candidate again changed the drawing:\n%s", screen.Text())
	}

	screen.Press([]byte("j"))
	screen.Press([]byte("j"))
	moved := screen.Text()
	assertRow(t, moved, cache.Path, "> [ ]", "4.0 KiB", "Cache", "2026-08-01 00:00:00Z")
	if !strings.Contains(moved, string(cache.Rule.Reason)) {
		t.Errorf("the highlighted Shared cache's Reason is missing:\n%s", moved)
	}
	assertTotal(t, moved, "1 Candidate, 1.0 KiB picked")

	if quit := screen.Press([]byte("t")); quit {
		t.Fatal("t quit")
	}
	ticked := screen.Text()
	assertRow(t, ticked, cache.Path, "> [x]", "4.0 KiB", "Cache", "2026-08-01 00:00:00Z")
	assertTotal(t, ticked, "2 Candidates, 5.0 KiB picked")

	screen.Press([]byte("u"))
	assertRow(t, screen.Text(), cache.Path, "> [ ]", "4.0 KiB", "Cache", "2026-08-01 00:00:00Z")
	assertTotal(t, screen.Text(), "1 Candidate, 1.0 KiB picked")

	screen.Press([]byte("\x1bOB"))
	beforeDocker := screen.Text()
	if !strings.Contains(beforeDocker, "> Docker data") {
		t.Fatalf("Docker data is not highlighted:\n%s", beforeDocker)
	}
	if screen.Press([]byte("t")) || screen.Press([]byte("u")) || screen.Text() != beforeDocker {
		t.Fatalf("Docker data accepted a tick:\n%s", screen.Text())
	}
	if screen.Press([]byte("j")) || screen.Text() != beforeDocker {
		t.Fatalf("j past the end moved:\n%s", screen.Text())
	}

	screen.Press([]byte("\x1b[A"))
	if !strings.Contains(lineContaining(t, screen.Text(), cache.Path), "> [ ]") {
		t.Fatalf("up did not move back to the Shared cache:\n%s", screen.Text())
	}
	beforeQuit := screen.Text()
	if !screen.Press([]byte("q")) {
		t.Fatal("q did not quit")
	}
	if screen.Text() != beforeQuit {
		t.Fatalf("quit changed the drawing:\n%s", screen.Text())
	}
}

func TestCtrlCQuitsWithoutChangingThePick(t *testing.T) {
	screen := tui.New(scan.Result{
		Candidates: []domain.Candidate{candidate("/app", "/app/node_modules", domain.Dependencies, "It comes back.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), true, false)},
		DockerData: domain.Unread("docker is not running"),
	})
	before := screen.Text()
	if !strings.Contains(before, "Docker data  docker is not running") {
		t.Fatalf("unread Docker data reads as:\n%s", before)
	}
	if !screen.Press([]byte{0x03}) {
		t.Fatal("ctrl-c did not quit")
	}
	if screen.Text() != before {
		t.Fatal("ctrl-c changed the drawing")
	}
}

func TestAnEmptyScanStillShowsDockerDataAndTheKeys(t *testing.T) {
	screen := tui.New(scan.Result{DockerData: domain.Reported(0)})
	text := screen.Text()
	if !strings.Contains(text, "No Candidates.\n") {
		t.Fatalf("text =\n%s\nwant No Candidates.", text)
	}
	if !strings.Contains(text, "> Docker data  0 B  reported only") {
		t.Fatalf("Docker data is not highlighted:\n%s", text)
	}
	assertTotal(t, text, "0 Candidates, 0 B picked")
	if !strings.Contains(text, "j/k move    t tick    u untick    q quit\n") {
		t.Errorf("the keys are missing:\n%s", text)
	}
}

func TestAPathThatCouldNotBeReadIsNamed(t *testing.T) {
	screen := tui.New(scan.Result{
		Candidates: []domain.Candidate{candidate("/app", "/app/node_modules", domain.Dependencies, "It comes back.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), true, false)},
		DockerData: domain.Reported(0),
		Unreadable: []scan.Unreadable{{Path: "/app/locked", Err: errors.New("permission denied")}},
	})
	text := screen.Text()
	if !strings.Contains(text, "Not read, so the list may be short and a Size may be low\n  /app/locked: permission denied\n") {
		t.Fatalf("the unreadable path is missing:\n%s", text)
	}
}

func TestAShortTerminalKeepsTheHighlightAndTheTotal(t *testing.T) {
	candidates := make([]domain.Candidate, 40)
	for i := range candidates {
		candidates[i] = candidate("/work/app", fmt.Sprintf("/work/app/out/%02d", i), domain.Cache, "The compiler writes it again.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), i == 0, false)
	}
	screen := tui.New(scan.Result{Candidates: candidates, DockerData: domain.Reported(0)})

	page := screen.Page(80, 12)
	if n := len(strings.Split(strings.TrimRight(page, "\n"), "\n")); n > 12 {
		t.Fatalf("%d lines, want at most 12\n%s", n, page)
	}
	if !strings.Contains(page, "> [x]") || !strings.Contains(page, filepath.FromSlash("out/00")) {
		t.Fatalf("the highlighted Candidate is off screen:\n%s", page)
	}
	if !strings.Contains(page, "1 Candidate, 1.0 KiB picked\n") || !strings.Contains(page, "q quit\n") || !strings.Contains(page, "Docker data") {
		t.Fatalf("the total, the keys or Docker data left the screen:\n%s", page)
	}

	for range candidates[1:] {
		screen.Press([]byte("j"))
	}
	page = screen.Page(80, 12)
	if !strings.Contains(page, "> [ ]") || !strings.Contains(page, filepath.FromSlash("out/39")) {
		t.Fatalf("the last Candidate is off screen:\n%s", page)
	}
	screen.Press([]byte("t"))
	if !strings.Contains(screen.Page(80, 12), "2 Candidates, 2.0 KiB picked\n") {
		t.Fatalf("the total did not follow the tick:\n%s", screen.Page(80, 12))
	}
	screen.Press([]byte("j"))
	if !strings.Contains(screen.Page(80, 12), "> Docker data") {
		t.Fatalf("Docker data is off screen:\n%s", screen.Page(80, 12))
	}
}

func TestAWideTerminalShowsTheWholeDrawing(t *testing.T) {
	screen := tui.New(scan.Result{
		Candidates: []domain.Candidate{candidate("/work/app", "/work/app/node_modules", domain.Dependencies, "It comes back.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), true, false)},
		DockerData: domain.Reported(0),
	})
	if screen.Page(200, 40) != screen.Text() || screen.Page(0, 0) != screen.Text() {
		t.Fatalf("page =\n%s\ntext =\n%s", screen.Page(200, 40), screen.Text())
	}
}

func TestANarrowTerminalKeepsTheTick(t *testing.T) {
	long := "/work/app/" + strings.Repeat("nested/", 20) + "node_modules"
	screen := tui.New(scan.Result{
		Candidates: []domain.Candidate{candidate("/work/app", long, domain.Dependencies, "It comes back.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), true, false)},
		DockerData: domain.Reported(1 << 30),
	})
	page := screen.Page(40, 20)
	for _, line := range strings.Split(strings.TrimRight(page, "\n"), "\n") {
		if len([]rune(line)) > 40 {
			t.Errorf("line is %d columns: %q", len([]rune(line)), line)
		}
	}
	if !strings.Contains(page, "> [x]") || !strings.Contains(page, "picked\n") {
		t.Fatalf("the tick or the total was clipped away:\n%s", page)
	}
}

func TestUnreadPathsLeaveRoomForTheHighlightAndTheTotal(t *testing.T) {
	unread := make([]scan.Unreadable, 20)
	for i := range unread {
		unread[i] = scan.Unreadable{Path: fmt.Sprintf("/locked/%02d", i), Err: errors.New("permission denied")}
	}
	screen := tui.New(scan.Result{
		Candidates: []domain.Candidate{candidate("/work/app", "/work/app/node_modules", domain.Dependencies, "It comes back.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), true, false)},
		DockerData: domain.Reported(0),
		Unreadable: unread,
	})
	for _, height := range []int{12, 24} {
		page := screen.Page(80, height)
		if n := len(strings.Split(strings.TrimRight(page, "\n"), "\n")); n > height {
			t.Errorf("height %d drew %d lines\n%s", height, n, page)
		}
		if !strings.Contains(page, "> [x]") || !strings.Contains(page, "node_modules") {
			t.Errorf("height %d lost the highlighted Candidate:\n%s", height, page)
		}
		if !strings.Contains(page, "1 Candidate, 1.0 KiB picked\n") {
			t.Errorf("height %d lost the total:\n%s", height, page)
		}
	}
	if screen.Page(200, 80) != screen.Text() {
		t.Fatalf("a tall terminal dropped part of the drawing:\n%s", screen.Page(200, 80))
	}
	short := screen.Page(80, 12)
	if !strings.Contains(short, "/work/app\n") {
		t.Fatalf("the Project heading was dropped:\n%s", short)
	}
	if !strings.Contains(short, "Not read, so the list may be short and a Size may be low") {
		t.Fatalf("the unread header was dropped:\n%s", short)
	}
	shown, more := 0, -1
	for _, line := range strings.Split(short, "\n") {
		if strings.HasPrefix(line, "  /locked/") {
			shown++
		}
		var n int
		if _, err := fmt.Sscanf(line, "  and %d more", &n); err == nil {
			more = n
		}
	}
	if more < 0 {
		more = 0
	}
	if shown+more != len(unread) {
		t.Fatalf("unread paths shown %d plus %d more, want %d\n%s", shown, more, len(unread), short)
	}
}

func TestUnreadPathsDoNotHideProjectHeadings(t *testing.T) {
	unread := make([]scan.Unreadable, 25)
	for i := range unread {
		unread[i] = scan.Unreadable{Path: fmt.Sprintf("/locked/%02d", i), Err: errors.New("permission denied")}
	}
	when := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	screen := tui.New(scan.Result{
		Candidates: []domain.Candidate{
			candidate("/work/app", "/work/app/node_modules", domain.Dependencies, "First.", 12288, when, true, false),
			candidate("/work/two", "/work/two/node_modules", domain.Dependencies, "Second.", 12288, when, true, false),
		},
		DockerData: domain.Reported(0),
		Unreadable: unread,
	})
	page := screen.Page(80, 23)
	if !strings.Contains(page, "/work/app\n") || !strings.Contains(page, "/work/two\n") {
		t.Fatalf("a Project heading is missing:\n%s", page)
	}
	screen.Press([]byte("j"))
	page = screen.Page(80, 23)
	two := strings.Index(page, "/work/two\n")
	if two < 0 || !strings.Contains(page[two:], "> [x]") || !strings.Contains(page, "/work/app\n") {
		t.Fatalf("the second Project cannot be told from the first:\n%s", page)
	}
}

func TestAProjectNamedDockerDataKeepsItsHeading(t *testing.T) {
	when := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	var candidates []domain.Candidate
	for _, name := range []string{"Docker data/c", "Docker data/d", "Docker data/e", "a", "b", "f"} {
		project := "/work/" + name
		candidates = append(candidates, candidate(project, project+"/node_modules", domain.Dependencies, "It comes back.", 12288, when, true, false))
	}
	screen := tui.New(scan.Result{Candidates: candidates, DockerData: domain.Reported(0)})
	page := screen.Page(100, 12)
	if !strings.Contains(page, "/work/Docker data/c\n") || !strings.Contains(page, "> [x]") {
		t.Fatalf("the Project heading was treated as Docker data:\n%s", page)
	}
	screen.Press([]byte("j"))
	page = screen.Page(100, 12)
	if !strings.Contains(page, "/work/Docker data/d\n") {
		t.Fatalf("the next Project heading was dropped:\n%s", page)
	}
}

func TestDockerStaysHighlightedOnAShortPane(t *testing.T) {
	screen := tui.New(scan.Result{
		Candidates: []domain.Candidate{candidate("/work/app", "/work/app/node_modules", domain.Dependencies, "It comes back.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), true, false)},
		DockerData: domain.Reported(0),
	})
	if screen.Press([]byte("j")) {
		t.Fatal("j quit")
	}
	page := screen.Page(80, 7)
	if !strings.Contains(page, "> Docker data") {
		t.Fatalf("Docker data left the screen:\n%s", page)
	}
	if !strings.Contains(page, "picked\n") {
		t.Fatalf("the total left the screen:\n%s", page)
	}
}

func TestABurstOfKeysIsAppliedOneAtATime(t *testing.T) {
	screen := tui.New(scan.Result{
		Candidates: []domain.Candidate{
			candidate("/work/app", "/work/app/one", domain.Dependencies, "First.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), false, false),
			candidate("/work/app", "/work/app/two", domain.Dependencies, "Second.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), false, false),
			candidate("/work/app", "/work/app/three", domain.Cache, "Third.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), false, false),
		},
		DockerData: domain.Reported(0),
	})
	var keys tui.Decoder
	if apply(&keys, &screen, []byte("jj")) {
		t.Fatal("jj quit")
	}
	if !strings.Contains(lineContaining(t, screen.Text(), "three"), "> [ ]") {
		t.Fatalf("jj did not move twice:\n%s", screen.Text())
	}

	one := tui.New(scan.Result{
		Candidates: []domain.Candidate{candidate("/work/app", "/work/app/node_modules", domain.Dependencies, "It comes back.", 1024, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), false, false)},
		DockerData: domain.Reported(0),
	})
	keys = tui.Decoder{}
	if !apply(&keys, &one, []byte("tq")) {
		t.Fatal("q in tq was dropped")
	}
	if !strings.Contains(one.Text(), "> [x]") {
		t.Fatalf("t in tq was dropped:\n%s", one.Text())
	}
}

func TestAnUnfinishedEscapeIsKeptForTheNextRead(t *testing.T) {
	var keys tui.Decoder
	if got := keys.Push([]byte{0x1b}); len(got) != 0 {
		t.Fatalf("unfinished escape produced %q", got)
	}
	if got := keys.Push([]byte{'['}); len(got) != 0 {
		t.Fatalf("unfinished escape produced %q", got)
	}
	got := keys.Push([]byte{'A', 'j'})
	if len(got) != 2 || string(got[0]) != "\x1b[A" || string(got[1]) != "j" {
		t.Fatalf("keys = %q, want up then j", got)
	}
}

func apply(keys *tui.Decoder, screen *tui.Screen, chunk []byte) bool {
	for _, key := range keys.Push(chunk) {
		if screen.Press(key) {
			return true
		}
	}
	return false
}

func TestAnUnknownKeyChangesNothing(t *testing.T) {
	screen := tui.New(scan.Result{DockerData: domain.Reported(0)})
	before := screen.Text()
	if screen.Press([]byte("x")) || screen.Press([]byte{0x1b}) || screen.Text() != before {
		t.Fatal("an unknown key changed the drawing or quit")
	}
}

func candidate(project, path string, kind domain.Kind, reason string, size domain.Size, last time.Time, stale, shared bool) domain.Candidate {
	rule := domain.Rule{Kind: kind, Reason: domain.Reason(reason), Where: domain.InProject{Manifest: "package.json", Candidate: "node_modules"}}
	if shared {
		rule.Where = domain.InSharedCache{Linux: ".npm/_cacache"}
	}
	return domain.Candidate{Path: path, Size: size, Ecosystem: "Node", Rule: rule, Project: project, LastUsed: last, Stale: stale}
}

func assertRow(t *testing.T, text, path, tick, size, kind, last string) {
	t.Helper()
	row := lineContaining(t, text, path)
	for _, want := range []string{tick, size, kind, last} {
		if !strings.Contains(row, want) {
			t.Errorf("row %q does not contain %q", row, want)
		}
	}
}

func assertTotal(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want+"\n") {
		t.Errorf("text =\n%s\nwant total %q", text, want)
	}
}

func lineContaining(t *testing.T, text, part string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, part) {
			return line
		}
	}
	t.Fatalf("no line contains %q\n%s", part, text)
	return ""
}

func between(t *testing.T, text, start, end string) string {
	t.Helper()
	from := strings.Index(text, start)
	if from < 0 {
		t.Fatalf("missing %q\n%s", start, text)
	}
	from += len(start)
	to := strings.Index(text[from:], end)
	if to < 0 {
		t.Fatalf("missing %q after %q\n%s", end, start, text)
	}
	return text[from : from+to]
}
