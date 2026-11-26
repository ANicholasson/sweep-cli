package clean_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/clean"
	"github.com/ANicholasson/sweep-cli/internal/domain/ecosystems"
	"github.com/ANicholasson/sweep-cli/internal/domain/plan"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestACleanMovesOnlyWhatThePlanNames(t *testing.T) {
	files, built := planned(t)
	bin := sweeptest.NewTrash(files)

	got := clean.Run(files, sweeptest.Git{}, bin, built)

	if len(got.Trashed) != 2 || len(got.Skipped) != 0 || len(got.Failed) != 0 {
		t.Fatalf("trashed %d, skipped %d, failed %d, want both Candidates moved", len(got.Trashed), len(got.Skipped), len(got.Failed))
	}
	if len(bin.Moved) != 2 {
		t.Fatalf("moved %v, want both Candidates", bin.Moved)
	}
}

func TestALinkAboveTheCandidateIsASkipAndNothingInsideIsTouched(t *testing.T) {
	files, built := planned(t)
	app := picked(t, built, "/app/node_modules")
	files.AsLink("app")
	bin := sweeptest.NewTrash(files)

	got := clean.Run(files, sweeptest.Git{}, bin, built)

	skip := onlySkip(t, got, app)
	if skip.Rule != clean.Root {
		t.Errorf("rule = %s, want the Root rule", skip.Rule)
	}
	if moved(bin, app) {
		t.Errorf("moved %s, which is reached through a Link", app)
	}
	if _, err := files.List(app); err != nil {
		t.Errorf("listing %s: %v, and a Skip must not touch what is inside", app, err)
	}
	if !moved(bin, picked(t, built, "/other/node_modules")) {
		t.Error("the other Candidate was not moved")
	}
}

func TestAnUnreadableDirectoryIsASkipUnderTheGitRule(t *testing.T) {
	files, built := planned(t)
	app := picked(t, built, "/app/node_modules")
	files.Unreadable("app/node_modules/sealed", errors.New("permission denied"))
	bin := sweeptest.NewTrash(files)

	got := clean.Run(files, sweeptest.Git{}, bin, built)

	skip := onlySkip(t, got, app)
	if skip.Rule != clean.Git {
		t.Errorf("rule = %s, want the Git rule", skip.Rule)
	}
	if moved(bin, app) {
		t.Errorf("moved %s, which holds a directory that cannot be read", app)
	}
	if _, err := files.Space(sweeptest.Top + "/app/node_modules/sealed/tracked.txt"); err != nil {
		t.Errorf("the file inside the unreadable directory: %v", err)
	}
}

func TestATrackedPathIsASkipUnderTheGitRule(t *testing.T) {
	files, built := planned(t)
	app := picked(t, built, "/app/node_modules")
	kept := sweeptest.NewGit(sweeptest.Top, sweeptest.Work{
		Files:   []string{"app/node_modules/keep.txt", "other/node_modules/a.js"},
		Tracked: []string{"app/node_modules/keep.txt"},
	})
	bin := sweeptest.NewTrash(files)

	got := clean.Run(files, kept, bin, built)

	skip := onlySkip(t, got, app)
	if skip.Rule != clean.Git {
		t.Errorf("rule = %s, want the Git rule", skip.Rule)
	}
	if moved(bin, app) {
		t.Error("moved a Candidate the Git rule keeps")
	}
}

func TestAChangedFingerprintIsASkip(t *testing.T) {
	files, built := planned(t)
	app := picked(t, built, "/app/node_modules")
	files.Touch("app/node_modules/keep.txt", time.Now())
	bin := sweeptest.NewTrash(files)

	got := clean.Run(files, sweeptest.Git{}, bin, built)

	skip := onlySkip(t, got, app)
	if skip.Rule != clean.Fingerprint {
		t.Errorf("rule = %s, want the Fingerprint rule", skip.Rule)
	}
	if moved(bin, app) {
		t.Error("moved a Candidate whose Fingerprint changed")
	}
}

func TestAPathOutsideTheRootsIsASkip(t *testing.T) {
	files, built := planned(t)
	built.Roots = []domain.Root{"/elsewhere"}
	bin := sweeptest.NewTrash(files)

	got := clean.Run(files, sweeptest.Git{}, bin, built)

	if len(got.Skipped) != len(built.Picked) || len(got.Trashed) != 0 || len(bin.Moved) != 0 {
		t.Fatalf("trashed %d, skipped %d, moved %v, want every Candidate kept by the Root rule", len(got.Trashed), len(got.Skipped), bin.Moved)
	}
	for _, skip := range got.Skipped {
		if skip.Rule != clean.Root {
			t.Errorf("%s is a %s, want the Root rule", skip.Path, skip.Rule)
		}
	}
}

func TestAFailureLeavesTheCandidateAndTheRestContinue(t *testing.T) {
	files, built := planned(t)
	app := picked(t, built, "/app/node_modules")
	other := picked(t, built, "/other/node_modules")
	bin := sweeptest.NewTrash(files)
	bin.Fail = map[string]error{app: errors.New("the trash refused it")}

	got := clean.Run(files, sweeptest.Git{}, bin, built)

	if len(got.Failed) != 1 || got.Failed[0].Path != app {
		t.Fatalf("failed = %+v, want %s", got.Failed, app)
	}
	if moved(bin, app) {
		t.Error("a failed move removed the Candidate")
	}
	if _, err := files.List(app); err != nil {
		t.Errorf("the refused Candidate is gone: %v", err)
	}
	if !moved(bin, other) {
		t.Error("the failure stopped the rest of the Plan")
	}
}

func TestAChangeBetweenTheRecheckAndTheMoveIsRecorded(t *testing.T) {
	// The re-check has already passed when Move runs. Before changes the
	// Candidate in that window. The move still happens, and the test records
	// that window.
	files, built := planned(t)
	app := pick(t, built, "/app/node_modules")
	bin := sweeptest.NewTrash(files)
	saw := false
	bin.Before = func(p string) {
		if p != app.Path {
			return
		}
		files.Touch("app/node_modules/keep.txt", time.Now())
		same, err := plan.Match(files, app)
		if err != nil {
			t.Fatal(err)
		}
		if same {
			t.Fatal("the change did not land before the move")
		}
		saw = true
	}

	got := clean.Run(files, sweeptest.Git{}, bin, built)

	if !saw {
		t.Fatal("the change never landed")
	}
	if !moved(bin, app.Path) {
		t.Fatal("the Candidate was not moved after the re-check")
	}
	for _, skip := range got.Skipped {
		if skip.Path == app.Path {
			t.Fatalf("the Candidate was a Skip under the %s after the re-check had passed", skip.Rule)
		}
	}
}

func planned(t *testing.T) (*sweeptest.Files, plan.Plan) {
	t.Helper()
	files := sweeptest.NewFiles(sweeptest.Tree{
		"app/package.json":                    sweeptest.File{Bytes: 20},
		"app/src/index.js":                    sweeptest.File{Bytes: 20},
		"app/node_modules/keep.txt":           sweeptest.File{Bytes: 20},
		"app/node_modules/sealed/tracked.txt": sweeptest.File{Bytes: 20},
		"other/package.json":                  sweeptest.File{Bytes: 20},
		"other/node_modules/a.js":             sweeptest.File{Bytes: 20},
	})
	found := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, ecosystems.All(), time.Now(), scan.Settings{})
	built, _, err := plan.Build(files, found, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Picked) != 2 {
		t.Fatalf("%d Candidates in the Plan, want app and other", len(built.Picked))
	}
	return files, built
}

func picked(t *testing.T, built plan.Plan, suffix string) string {
	t.Helper()
	return pick(t, built, suffix).Path
}

func pick(t *testing.T, built plan.Plan, suffix string) plan.Picked {
	t.Helper()
	for _, c := range built.Picked {
		if strings.HasSuffix(c.Path, suffix) {
			return c
		}
	}
	t.Fatalf("no Candidate ending in %s in %+v", suffix, built.Picked)
	return plan.Picked{}
}

func onlySkip(t *testing.T, got clean.Result, path string) clean.Skip {
	t.Helper()
	var found clean.Skip
	n := 0
	for _, skip := range got.Skipped {
		if skip.Path == path {
			found = skip
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%s was skipped %d times in %+v", path, n, got.Skipped)
	}
	return found
}

func moved(bin *sweeptest.Trash, path string) bool {
	for _, p := range bin.Moved {
		if p == path {
			return true
		}
	}
	return false
}
