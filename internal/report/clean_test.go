package report_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/clean"
	"github.com/ANicholasson/sweep-cli/internal/report"
)

func TestCleanReportsMovedSkippedAndFailed(t *testing.T) {
	var out bytes.Buffer
	err := report.Clean(&out, clean.Result{
		Trashed: []clean.Item{{Path: "/work/old", Size: 2048}},
		Skipped: []clean.Skip{{
			Item: clean.Item{Path: "/work/changed", Size: 512},
			Rule: clean.Fingerprint,
		}},
		Failed: []clean.Failure{{
			Item: clean.Item{Path: "/work/stuck", Size: 1024},
			Err:  errors.New("the trash refused it"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"Moved to trash\n",
		"/work/old",
		"Skipped\n",
		"Fingerprint rule",
		"/work/changed",
		"Failed\n",
		"/work/stuck",
		"the trash refused it",
		"Moved to trash: 1 Candidate, 2.0 KiB\n",
		"Skipped 1 Candidate, 512 B\n",
		"Failed 1 Candidate, 1.0 KiB\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is\n%s\nwant it to contain %q", got, want)
		}
	}
	if strings.Contains(strings.ToLower(got), "reclaimed") {
		t.Errorf("trashed space is called reclaimed:\n%s", got)
	}
}

func TestCleanSaysWhenNothingMoved(t *testing.T) {
	var out bytes.Buffer
	err := report.Clean(&out, clean.Result{
		Skipped: []clean.Skip{{
			Item: clean.Item{Path: "/work/kept", Size: domain.Size(1)},
			Rule: clean.Root,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Moved to trash: nothing\n") || !strings.Contains(got, "Root rule") {
		t.Errorf("report is\n%s\nwant nothing moved and the Root rule", got)
	}
	if strings.Contains(strings.ToLower(got), "reclaimed") {
		t.Errorf("trashed space is called reclaimed:\n%s", got)
	}
}
