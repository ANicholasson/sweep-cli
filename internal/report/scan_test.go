package report_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/report"
)

func TestTheTableWritesASizeInTheLargestUnitBelowIt(t *testing.T) {
	for size, want := range map[domain.Size]string{
		0:             "0 B",
		1023:          "1023 B",
		1024:          "1.0 KiB",
		86016:         "84.0 KiB",
		5 << 20:       "5.0 MiB",
		3<<30 + 1<<29: "3.5 GiB",
		2 << 40:       "2.0 TiB",
		2048 << 40:    "2048.0 TiB",
	} {
		var out strings.Builder
		result := scan.Result{Candidates: []domain.Candidate{{Path: "/p/node_modules", Size: size, Rule: domain.Rule{Kind: domain.Dependencies, Reason: "It comes back."}}}}
		if err := report.Scan(&out, result, report.Table); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "\n"+want+"  ") {
			t.Errorf("Size %d reads as\n%s\nwant %q", size, out.String(), want)
		}
	}
}

func TestBothFormatsNameWhatWasNotRead(t *testing.T) {
	result := scan.Result{
		Roots:      []domain.Root{"/p"},
		Unreadable: []scan.Unreadable{{Path: "/p/locked", Err: errors.New("permission denied")}},
	}

	var table strings.Builder
	if err := report.Scan(&table, result, report.Table); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(table.String(), "/p/locked: permission denied") {
		t.Errorf("the table is\n%s\nwant /p/locked and why in it", table.String())
	}

	var out strings.Builder
	if err := report.Scan(&out, result, report.JSON); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Unreadable []struct{ Path, Error string }
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Unreadable) != 1 || got.Unreadable[0].Path != "/p/locked" || got.Unreadable[0].Error != "permission denied" {
		t.Errorf("unreadable = %+v, want /p/locked with its error", got.Unreadable)
	}
}
