package clean_test

import (
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

func TestAChangeBetweenTheRecheckAndTheMoveIsRecorded(t *testing.T) {
	// The binary cannot show this. The change has to land after the
	// re-check has passed and before the move, inside one run, and a run
	// of the binary has no way to change the Candidate in that gap. The
	// fake's Move is the only place the change can be injected. The move
	// still happens.
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
		"app/package.json":          sweeptest.File{Bytes: 20},
		"app/src/index.js":          sweeptest.File{Bytes: 20},
		"app/node_modules/keep.txt": sweeptest.File{Bytes: 20},
		"other/package.json":        sweeptest.File{Bytes: 20},
		"other/node_modules/a.js":   sweeptest.File{Bytes: 20},
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

func moved(bin *sweeptest.Trash, path string) bool {
	for _, p := range bin.Moved {
		if p == path {
			return true
		}
	}
	return false
}
