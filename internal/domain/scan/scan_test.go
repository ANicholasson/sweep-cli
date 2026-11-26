package scan_test

import (
	"errors"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

// scanNow is the clock the tests hand a Scan, so Stale does not follow the machine.
var scanNow = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

var defaultSettings = scan.Settings{StaleAfter: domain.StaleAfter}

var (
	gradle = domain.Ecosystem{Name: "Gradle", Rules: []domain.Rule{
		{Kind: domain.BuildOutput, Reason: "gradle build makes it again.", Where: domain.InProject{Manifest: "build.gradle", Candidate: "build"}},
	}}
	web = domain.Ecosystem{Name: "Web", Rules: []domain.Rule{
		{Kind: domain.Dependencies, Reason: "An install puts it back.", Where: domain.InProject{Manifest: "package.json", Candidate: "node_modules"}},
		{Kind: domain.BuildOutput, Reason: "A build makes it again.", Where: domain.InProject{Manifest: "package.json", Candidate: "build"}},
	}}
)

func TestAPathTwoRulesRecogniseIsListedOnceUnderTheSameRule(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		"app/package.json":  sweeptest.File{Bytes: 1},
		"app/build.gradle":  sweeptest.File{Bytes: 1},
		"app/build/out.bin": sweeptest.File{Bytes: 1},
	})
	for _, ecosystems := range [][]domain.Ecosystem{{gradle, web}, {web, gradle}} {
		got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, ecosystems, scanNow, defaultSettings)

		if len(got.Candidates) != 1 {
			t.Fatalf("%d Candidates, want app/build once", len(got.Candidates))
		}
		if c := got.Candidates[0]; c.Path != sweeptest.Top+"/app/build" || c.Ecosystem != "Gradle" || c.Rule.Reason != gradle.Rules[0].Reason {
			t.Errorf("listed %s under %s, %q, want app/build under Gradle, whose Name sorts first", c.Path, c.Ecosystem, c.Rule.Reason)
		}
	}
}

func TestSizeCountsEachDirectoryAndEachHardLinkedFileOnce(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		"app/package.json":           sweeptest.File{Bytes: 1},
		"app/node_modules/a/LICENSE": sweeptest.File{Bytes: 64 << 10},
		"app/node_modules/b/LICENSE": sweeptest.HardLink{To: "app/node_modules/a/LICENSE"},
	})

	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web}, scanNow, defaultSettings)

	want := domain.Size(3*sweeptest.DirSpace + 64<<10) // node_modules, a and b, and the LICENSE once
	if len(got.Candidates) != 1 || got.Candidates[0].Size != want {
		t.Errorf("Candidates = %+v, want one of Size %d", got.Candidates, want)
	}
}

func TestWhatCannotBeReadIsReportedOnceAndTheRestIsStillFound(t *testing.T) {
	denied := errors.New("permission denied")
	files := sweeptest.NewFiles(sweeptest.Tree{
		"locked/app/package.json":         sweeptest.File{Bytes: 1},
		"locked/app/node_modules/a/x.js":  sweeptest.File{Bytes: 1},
		"open/app/package.json":           sweeptest.File{Bytes: 1},
		"open/app/node_modules/a/x.js":    sweeptest.File{Bytes: 1},
		"open/app/node_modules/sealed/y":  sweeptest.File{Bytes: 4096},
		"open/app/node_modules/sealed/zz": sweeptest.File{Bytes: 4096},
	})
	files.Unreadable("locked", denied)
	files.Unreadable("open/app/node_modules/sealed", denied)

	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top, "/gone"}, []domain.Ecosystem{web}, scanNow, defaultSettings)

	want := domain.Size(2*sweeptest.DirSpace + 512) // node_modules and a, and x.js
	if len(got.Candidates) != 1 || got.Candidates[0].Path != sweeptest.Top+"/open/app/node_modules" || got.Candidates[0].Size != want {
		t.Errorf("Candidates = %+v, want open/app/node_modules alone, of Size %d", got.Candidates, want)
	}
	var paths []string
	for _, u := range got.Unreadable {
		if !errors.Is(u.Err, denied) && u.Path != "/gone" {
			t.Errorf("%s is unreadable with %v, want the error the files gave", u.Path, u.Err)
		}
		paths = append(paths, u.Path)
	}
	wantPaths := []string{sweeptest.Top + "/locked", sweeptest.Top + "/open/app/node_modules/sealed", "/gone"}
	if !slices.Equal(paths, wantPaths) {
		t.Errorf("Unreadable = %v, want %v", paths, wantPaths)
	}
}

func TestAScanNeverEntersALinkAndNeverListsOne(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		"real/app/package.json":                 sweeptest.File{Bytes: 1},
		"real/app/node_modules/a/x.js":          sweeptest.File{Bytes: 1},
		"real/app/node_modules/.bin/a-tool":     sweeptest.Link{To: "elsewhere"},
		"elsewhere/big":                         sweeptest.File{Bytes: 1 << 20},
		"alias":                                 sweeptest.Link{To: "real"},
		"linked/package.json":                   sweeptest.File{Bytes: 1},
		"linked/node_modules":                   sweeptest.Link{To: "real/app/node_modules"},
		"elsewhere/package.json":                sweeptest.File{Bytes: 1},
		"elsewhere/node_modules/b/package.json": sweeptest.File{Bytes: 1},
	})

	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top + "/real"}, []domain.Ecosystem{web}, scanNow, defaultSettings)

	want := domain.Size(3*sweeptest.DirSpace + 512) // node_modules, a and .bin, and x.js, while the Link a-tool takes none
	if len(got.Candidates) != 1 || got.Candidates[0].Path != sweeptest.Top+"/real/app/node_modules" || got.Candidates[0].Size != want {
		t.Errorf("Candidates = %+v, want real/app/node_modules alone, of Size %d", got.Candidates, want)
	}
	if len(got.Unreadable) != 0 {
		t.Errorf("Unreadable = %v, want nothing", got.Unreadable)
	}

	got = scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web}, scanNow, defaultSettings)

	var paths []string
	for _, c := range got.Candidates {
		paths = append(paths, c.Path)
	}
	wantPaths := []string{sweeptest.Top + "/elsewhere/node_modules", sweeptest.Top + "/real/app/node_modules"}
	if !slices.Equal(paths, wantPaths) || len(got.Unreadable) != 0 {
		t.Errorf("Candidates = %v, Unreadable = %v, want %v and nothing unreadable", paths, got.Unreadable, wantPaths)
	}
}

// offered names a Candidate outside the Root and a Link a Rule would
// recognise. The fake cannot: its tree stays beneath its top, and a Link
// there has no manifest beside it.
type offered struct{ n uint64 }

func (o *offered) List(dir string) ([]scan.Entry, error) {
	switch dir {
	case sweeptest.Top:
		return []scan.Entry{
			{Name: "package.json", Path: sweeptest.Top + "/package.json", Is: scan.File},
			{Name: "node_modules", Path: "/outside/node_modules", Is: scan.Dir},
			{Name: "app", Path: sweeptest.Top + "/app", Is: scan.Dir},
		}, nil
	case sweeptest.Top + "/app":
		return []scan.Entry{
			{Name: "package.json", Path: sweeptest.Top + "/app/package.json", Is: scan.File},
			{Name: "node_modules", Path: sweeptest.Top + "/app/node_modules", Is: scan.Link},
		}, nil
	default:
		return nil, errors.New(dir)
	}
}

func (o *offered) Space(string) (scan.Space, error) {
	o.n++
	return scan.Space{Bytes: sweeptest.DirSpace, File: scan.FileID{Volume: 1, File: o.n}}, nil
}

func (*offered) Modified(string) (time.Time, error) { return time.Time{}, nil }

func TestAScanDropsAPathOutsideTheRootAndALink(t *testing.T) {
	got := scan.Scan(&offered{}, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web}, scanNow, defaultSettings)

	if len(got.Candidates) != 0 || len(got.Unreadable) != 0 {
		t.Errorf("Candidates = %+v, Unreadable = %v, want neither", got.Candidates, got.Unreadable)
	}
}

// namedWithABackslash is a directory whose name contains a backslash.
// On Linux and macOS that is a legal name. The fake's paths are
// slash-separated and cannot spell it.
type namedWithABackslash struct{ n uint64 }

func (o *namedWithABackslash) List(dir string) ([]scan.Entry, error) {
	const held = sweeptest.Top + `/\..`
	switch dir {
	case sweeptest.Top:
		return []scan.Entry{{Name: `\..`, Path: held, Is: scan.Dir}}, nil
	case held:
		return []scan.Entry{
			{Name: "package.json", Path: held + "/package.json", Is: scan.File},
			{Name: "node_modules", Path: held + "/node_modules", Is: scan.Dir},
		}, nil
	default:
		return nil, errors.New(dir)
	}
}

func (o *namedWithABackslash) Space(string) (scan.Space, error) {
	o.n++
	return scan.Space{Bytes: sweeptest.DirSpace, File: scan.FileID{Volume: 1, File: o.n}}, nil
}

func (*namedWithABackslash) Modified(string) (time.Time, error) { return time.Time{}, nil }

func TestAScanKeepsAPosixNameThatContainsABackslash(t *testing.T) {
	got := scan.Scan(&namedWithABackslash{}, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web}, scanNow, defaultSettings)

	want := sweeptest.Top + `/\../node_modules`
	if len(got.Candidates) != 1 || got.Candidates[0].Path != want {
		t.Errorf("Candidates = %+v, want %s", got.Candidates, want)
	}
}

// mounted is a Root Readlink resolves to a Windows volume path. The fake
// cannot build a mount point.
const volumeRoot = `\\?\Volume{11111111-2222-3333-4444-555555555555}\`

type mounted struct{ n uint64 }

func (m *mounted) List(dir string) ([]scan.Entry, error) {
	switch dir {
	case volumeRoot:
		return []scan.Entry{
			{Name: "package.json", Path: volumeRoot + "package.json", Is: scan.File},
			{Name: "node_modules", Path: volumeRoot + "node_modules", Is: scan.Dir},
		}, nil
	case volumeRoot + "node_modules":
		return nil, nil
	default:
		return nil, errors.New(dir)
	}
}

func (m *mounted) Space(string) (scan.Space, error) {
	m.n++
	return scan.Space{Bytes: sweeptest.DirSpace, File: scan.FileID{Volume: 1, File: m.n}}, nil
}

func (*mounted) Modified(string) (time.Time, error) { return time.Time{}, nil }

func TestAScanOfAVolumeMountListsItsCandidates(t *testing.T) {
	got := scan.Scan(&mounted{}, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{volumeRoot}, []domain.Ecosystem{web}, scanNow, defaultSettings)

	want := volumeRoot + "node_modules"
	if len(got.Candidates) != 1 || got.Candidates[0].Path != want || len(got.Unreadable) != 0 {
		t.Errorf("Candidates = %+v, Unreadable = %v, want %s", got.Candidates, got.Unreadable, want)
	}
}

func TestTheGitRuleKeepsAPathOutOfTheCandidates(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		"app/package.json":                       sweeptest.File{Bytes: 1},
		"app/node_modules/keep.js":               sweeptest.File{Bytes: 1},
		"app/node_modules/pkg/package.json":      sweeptest.File{Bytes: 1},
		"app/node_modules/pkg/node_modules/a.js": sweeptest.File{Bytes: 1},
		"app/other/package.json":                 sweeptest.File{Bytes: 1},
		"app/other/node_modules/b.js":            sweeptest.File{Bytes: 1},
		"loose/package.json":                     sweeptest.File{Bytes: 1},
		"loose/node_modules/c.js":                sweeptest.File{Bytes: 1},
	})
	git := sweeptest.NewGit(sweeptest.Top, sweeptest.Work{
		Repo: "app",
		Files: []string{
			"app/package.json",
			"app/node_modules/keep.js",
			"app/node_modules/pkg/package.json",
			"app/node_modules/pkg/node_modules/a.js",
			"app/other/package.json",
			"app/other/node_modules/b.js",
			"loose/package.json",
			"loose/node_modules/c.js",
		},
		Tracked: []string{"app/package.json", "app/node_modules/keep.js"},
	})

	got := scan.Scan(files, git, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web}, scanNow, defaultSettings)

	// app/node_modules holds a tracked file, so the Git rule keeps it and
	// the Scan does not enter it. The node_modules inside it is absent.
	// loose is outside the work tree, so it is still a Candidate.
	var paths []string
	for _, c := range got.Candidates {
		paths = append(paths, c.Path)
	}
	want := []string{sweeptest.Top + "/app/other/node_modules", sweeptest.Top + "/loose/node_modules"}
	if !slices.Equal(paths, want) || len(got.Unreadable) != 0 {
		t.Errorf("Candidates = %v, Unreadable = %v, want %v and nothing unreadable", paths, got.Unreadable, want)
	}
}

func TestAPathGitCannotAnswerIsNotACandidate(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		"app/package.json":        sweeptest.File{Bytes: 1},
		"app/node_modules/a.js":   sweeptest.File{Bytes: 1},
		"loose/package.json":      sweeptest.File{Bytes: 1},
		"loose/node_modules/b.js": sweeptest.File{Bytes: 1},
	})
	git := sweeptest.NewGit(sweeptest.Top, sweeptest.Work{Broken: true})

	got := scan.Scan(files, git, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web}, scanNow, defaultSettings)

	if len(got.Candidates) != 0 || len(got.Unreadable) != 0 {
		t.Errorf("Candidates = %+v, Unreadable = %v, want no Candidate and nothing unreadable", got.Candidates, got.Unreadable)
	}
}

func TestLastUsedIgnoresCandidatesAndStaleIsOlderThanThirtyDays(t *testing.T) {
	old := scanNow.Add(-31 * 24 * time.Hour)
	fresh := scanNow.Add(-29 * 24 * time.Hour)
	edge := scanNow.Add(-domain.StaleAfter)
	justOlder := edge.Add(-time.Second)
	inside := scanNow.Add(-time.Hour)
	files := sweeptest.NewFiles(sweeptest.Tree{
		"stale/package.json":      sweeptest.File{Bytes: 1, Modified: old},
		"stale/src/app.js":        sweeptest.File{Bytes: 1, Modified: old},
		"stale/node_modules/a.js": sweeptest.File{Bytes: 1, Modified: inside},
		"stale/link":              sweeptest.Link{To: "outside"},
		"outside/new.js":          sweeptest.File{Bytes: 1, Modified: inside},
		"fresh/package.json":      sweeptest.File{Bytes: 1, Modified: old},
		"fresh/src/app.js":        sweeptest.File{Bytes: 1, Modified: fresh},
		"fresh/node_modules/a.js": sweeptest.File{Bytes: 1, Modified: inside},
		"fresh/build/out.js":      sweeptest.File{Bytes: 1, Modified: old},
		"edge/package.json":       sweeptest.File{Bytes: 1, Modified: edge},
		"edge/node_modules/a.js":  sweeptest.File{Bytes: 1, Modified: old},
		"older/package.json":      sweeptest.File{Bytes: 1, Modified: justOlder},
		"older/node_modules/a.js": sweeptest.File{Bytes: 1, Modified: inside},
	})

	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web}, scanNow, defaultSettings)

	want := map[string]struct {
		used  time.Time
		stale bool
	}{
		sweeptest.Top + "/stale/node_modules": {old, true},
		sweeptest.Top + "/fresh/node_modules": {fresh, false},
		sweeptest.Top + "/fresh/build":        {fresh, false},
		sweeptest.Top + "/edge/node_modules":  {edge, false},
		sweeptest.Top + "/older/node_modules": {justOlder, true},
	}
	if len(got.Candidates) != len(want) {
		t.Fatalf("Candidates = %+v, want %d", got.Candidates, len(want))
	}
	for _, c := range got.Candidates {
		w, ok := want[c.Path]
		if !ok {
			t.Errorf("%s is listed, and it should not be", c.Path)
			continue
		}
		if c.Project != path.Dir(c.Path) || !c.LastUsed.Equal(w.used) || c.Stale != w.stale {
			t.Errorf("%s: Project %s, Last used %s, Stale %v, want %s, %s, %v", c.Path, c.Project, c.LastUsed, c.Stale, path.Dir(c.Path), w.used, w.stale)
		}
	}
	if len(got.Unreadable) != 0 {
		t.Errorf("Unreadable = %v, want nothing", got.Unreadable)
	}
}

func TestDockerDataIsReportedAndNeverBecomesACandidate(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		"app/package.json":      sweeptest.File{Bytes: 1},
		"app/node_modules/a.js": sweeptest.File{Bytes: 1},
	})
	roots := []domain.Root{sweeptest.Top}
	ecosystems := []domain.Ecosystem{web}

	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{Answer: domain.Reported(42)}, roots, ecosystems, scanNow, defaultSettings)

	size, ok := got.DockerData.Size()
	if !ok || size != 42 || got.DockerData.Mark() != "reported only" {
		t.Errorf("Docker data = %d, %v, %q, want 42 reported only", size, ok, got.DockerData.Mark())
	}
	if len(got.Candidates) != 1 || got.Candidates[0].Path != sweeptest.Top+"/app/node_modules" {
		t.Errorf("Candidates = %+v, want the node_modules only", got.Candidates)
	}

	got = scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{Answer: domain.Unread("docker is not running")}, roots, ecosystems, scanNow, defaultSettings)

	if _, ok := got.DockerData.Size(); ok || got.DockerData.Mark() != "docker is not running" {
		t.Errorf("Docker data mark = %q, want docker is not running and no size", got.DockerData.Mark())
	}
	if len(got.Unreadable) != 0 || len(got.Candidates) != 1 {
		t.Errorf("Candidates = %+v, Unreadable = %v, want the Candidate and nothing unreadable", got.Candidates, got.Unreadable)
	}
}

// TestAnIgnoredPathIsLeftOut is the part a binary cannot show on every
// operating system: a path the Scan could not read, beneath an ignored
// directory, is not reported. The Fixture test shows an ignored Candidate
// is absent from a real tree.
func TestAnIgnoredPathIsLeftOut(t *testing.T) {
	files := sweeptest.NewFiles(sweeptest.Tree{
		"keep/package.json":            sweeptest.File{Bytes: 1, Modified: scanNow.Add(-31 * 24 * time.Hour)},
		"keep/node_modules/a.js":       sweeptest.File{Bytes: 1},
		"keep-extra/package.json":      sweeptest.File{Bytes: 1, Modified: scanNow.Add(-time.Hour)},
		"keep-extra/node_modules/a.js": sweeptest.File{Bytes: 1},
		"drop/package.json":            sweeptest.File{Bytes: 1, Modified: scanNow.Add(-100 * 24 * time.Hour)},
		"drop/src/new.js":              sweeptest.File{Bytes: 1, Modified: scanNow.Add(-time.Minute)},
		"drop/node_modules/a.js":       sweeptest.File{Bytes: 1},
		"drop/sealed/x.js":             sweeptest.File{Bytes: 1},
	})
	files.Unreadable("drop/sealed", errors.New("permission denied"))

	got := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web}, scanNow, scan.Settings{
		StaleAfter: domain.StaleAfter,
		Ignore:     []string{sweeptest.Top + "/keep", sweeptest.Top + "/drop/src", sweeptest.Top + "/drop/sealed"},
	})

	want := sweeptest.Top + "/keep-extra/node_modules"
	drop := sweeptest.Top + "/drop/node_modules"
	if len(got.Candidates) != 2 {
		t.Fatalf("Candidates = %+v, want keep-extra, which only shares a prefix, and drop", got.Candidates)
	}
	var sawKeepExtra, sawDrop bool
	for _, c := range got.Candidates {
		switch c.Path {
		case want:
			sawKeepExtra = true
			if c.Stale {
				t.Errorf("%s is Stale, and its Last used is inside the Stale threshold", c.Path)
			}
		case drop:
			sawDrop = true
			if !c.LastUsed.Equal(scanNow.Add(-time.Minute)) || c.Stale {
				t.Errorf("%s Last used %s, Stale %v, want the ignored source file's time and not Stale", c.Path, c.LastUsed, c.Stale)
			}
		default:
			t.Errorf("%s is listed, and it is ignored or beneath an ignored path", c.Path)
		}
	}
	if !sawKeepExtra || !sawDrop {
		t.Errorf("Candidates = %+v, want %s and %s", got.Candidates, want, drop)
	}
	sealed := sweeptest.Top + "/drop/sealed"
	keep := sweeptest.Top + "/keep"
	for _, u := range got.Unreadable {
		if u.Path == sealed || u.Path == keep || strings.HasPrefix(u.Path, keep+"/") {
			t.Errorf("reported %s, which is ignored", u.Path)
		}
	}
}
