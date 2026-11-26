package plan_test

import (
	"testing"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/ecosystems"
	"github.com/ANicholasson/sweep-cli/internal/domain/plan"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestADirectoryCycleDoesNotHangTheFingerprint(t *testing.T) {
	// A directory that contains itself, such as a bind mount, cannot be
	// built on disk here. The fake gives the Candidate a second name with
	// the same file identity, which a walk that does not remember what it
	// entered would follow forever.
	files := sweeptest.NewFiles(sweeptest.Tree{
		"app/package.json":          sweeptest.File{Bytes: 1},
		"app/node_modules/keep.txt": sweeptest.File{Bytes: 1},
		"app/node_modules/again":    sweeptest.HardLink{To: "app/node_modules"},
	})
	found := scan.Scan(files, sweeptest.Git{}, sweeptest.Docker{}, []domain.Root{sweeptest.Top}, ecosystems.All(), time.Now(), scan.Settings{StaleAfter: domain.StaleAfter})
	if len(found.Candidates) != 1 {
		t.Fatalf("%d Candidates, want the one directory", len(found.Candidates))
	}

	built, _, err := plan.Build(files, found, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Picked) != 1 {
		t.Fatalf("%d picked, want the Candidate", len(built.Picked))
	}
	// keep.txt and again. again is the Candidate again, and is not entered.
	if built.Picked[0].Fingerprint.Entries != 2 {
		t.Errorf("entries = %d, want 2", built.Picked[0].Fingerprint.Entries)
	}
}
