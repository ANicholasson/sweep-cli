package scan_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

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
		got := scan.Scan(files, sweeptest.Git{}, []domain.Root{sweeptest.Top}, ecosystems)

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

	got := scan.Scan(files, sweeptest.Git{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web})

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

	got := scan.Scan(files, sweeptest.Git{}, []domain.Root{sweeptest.Top, "/gone"}, []domain.Ecosystem{web})

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

	got := scan.Scan(files, sweeptest.Git{}, []domain.Root{sweeptest.Top + "/real"}, []domain.Ecosystem{web})

	want := domain.Size(3*sweeptest.DirSpace + 512) // node_modules, a and .bin, and x.js, while the Link a-tool takes none
	if len(got.Candidates) != 1 || got.Candidates[0].Path != sweeptest.Top+"/real/app/node_modules" || got.Candidates[0].Size != want {
		t.Errorf("Candidates = %+v, want real/app/node_modules alone, of Size %d", got.Candidates, want)
	}
	if len(got.Unreadable) != 0 {
		t.Errorf("Unreadable = %v, want nothing", got.Unreadable)
	}

	got = scan.Scan(files, sweeptest.Git{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web})

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

func TestAScanDropsAPathOutsideTheRootAndALink(t *testing.T) {
	got := scan.Scan(&offered{}, sweeptest.Git{}, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web})

	if len(got.Candidates) != 0 || len(got.Unreadable) != 0 {
		t.Errorf("Candidates = %+v, Unreadable = %v, want neither", got.Candidates, got.Unreadable)
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

	got := scan.Scan(files, git, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web})

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

	got := scan.Scan(files, git, []domain.Root{sweeptest.Top}, []domain.Ecosystem{web})

	if len(got.Candidates) != 0 || len(got.Unreadable) != 0 {
		t.Errorf("Candidates = %+v, Unreadable = %v, want no Candidate and nothing unreadable", got.Candidates, got.Unreadable)
	}
}
