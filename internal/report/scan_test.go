package report_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

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

func TestTheTableGroupsCandidatesByProject(t *testing.T) {
	used := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	rule := domain.Rule{Kind: domain.Dependencies, Reason: "It comes back."}
	build := domain.Rule{Kind: domain.BuildOutput, Reason: "A build makes it again."}
	result := scan.Result{Roots: []domain.Root{"/work"}, Candidates: []domain.Candidate{
		{Path: "/work/app/node_modules", Size: 2048, Project: "/work/app", LastUsed: used, Stale: true, Rule: rule},
		{Path: "/work/fresh/node_modules", Size: 1024, Project: "/work/fresh", LastUsed: used.Add(24 * time.Hour), Rule: rule},
		{Path: "/work/app/build", Size: 512, Project: "/work/app", LastUsed: used, Stale: true, Rule: build},
		{Path: "/work/loose", Size: 100, LastUsed: used, Stale: true, Rule: rule},
	}}

	var out strings.Builder
	if err := report.Scan(&out, result, report.Table); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	app := strings.Index(text, "\n/work/app\n")
	fresh := strings.Index(text, "\n/work/fresh\n")
	none := strings.Index(text, "\n(no Project)\n")
	if app < 0 || fresh < app || none < fresh {
		t.Fatalf("groups are out of order in\n%s", text)
	}
	if strings.Count(text, "\n/work/app\n") != 1 {
		t.Errorf("the Project /work/app is headed more than once in\n%s", text)
	}
	appRows := text[app:fresh]
	if !strings.Contains(appRows, "/work/app/node_modules") || !strings.Contains(appRows, "/work/app/build") || !strings.Contains(appRows, " stale ") {
		t.Errorf("the Stale Project's rows are\n%s", appRows)
	}
	freshRows := text[fresh:none]
	if strings.Contains(freshRows, " stale ") || !strings.Contains(freshRows, "2026-01-03 03:04:05Z") {
		t.Errorf("the fresh Project's rows are\n%s", freshRows)
	}
	if !strings.Contains(text[none:], "/work/loose") || !strings.Contains(text[none:], " stale ") {
		t.Errorf("the Candidate with no Project is\n%s", text[none:])
	}
}

func TestJSONCarriesTheProjectLastUsedAndStale(t *testing.T) {
	used := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("CET", 3600))
	result := scan.Result{Candidates: []domain.Candidate{{
		Path: "/p/node_modules", Size: 10, Ecosystem: "Node", Project: "/p",
		LastUsed: used, Stale: true,
		Rule: domain.Rule{Kind: domain.Dependencies, Reason: "It comes back."},
	}}}

	var out strings.Builder
	if err := report.Scan(&out, result, report.JSON); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Candidates []struct {
			Project  string
			LastUsed time.Time `json:"last_used"`
			Stale    bool
		}
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].Project != "/p" || !got.Candidates[0].Stale || !got.Candidates[0].LastUsed.Equal(used) {
		t.Errorf("candidates = %+v, want project /p, the same instant, and stale", got.Candidates)
	}
	if !strings.Contains(out.String(), `"last_used": "2026-01-02T02:04:05Z"`) {
		t.Errorf("last_used is not UTC in\n%s", out.String())
	}
}

func TestDockerDataIsOneLineAndNotACandidate(t *testing.T) {
	result := scan.Result{
		Roots: []domain.Root{"/work"},
		Candidates: []domain.Candidate{{
			Path: "/work/app/node_modules",
			Size: 2048,
			Rule: domain.Rule{Kind: domain.Dependencies, Reason: "An install puts it back."},
		}},
		DockerData: domain.Reported(1536),
	}

	var table strings.Builder
	if err := report.Scan(&table, result, report.Table); err != nil {
		t.Fatal(err)
	}
	got := table.String()
	const line = "Docker data  1.5 KiB  reported only\n"
	if !strings.Contains(got, "\n\n"+line) {
		t.Fatalf("table is\n%s\nwant %q on its own line", got, line)
	}
	if strings.Count(got, "reported only") != 1 || strings.Count(got, "Docker data") != 1 {
		t.Fatalf("table is\n%s\nwant Docker data marked once", got)
	}
	before, _, _ := strings.Cut(got, "\n\nDocker data")
	if strings.Contains(before, "Docker data") || !strings.Contains(before, "/work/app/node_modules") {
		t.Fatalf("the Candidate table is\n%s", before)
	}

	var out strings.Builder
	if err := report.Scan(&out, result, report.JSON); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Candidates []struct {
			Path   string
			Kind   string
			Reason string
		}
		DockerData struct {
			Size *int64
			Mark string
		}
	}
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Candidates) != 1 || doc.Candidates[0].Kind != "Dependencies" || doc.Candidates[0].Reason != "An install puts it back." {
		t.Errorf("candidates = %+v, want the node_modules only", doc.Candidates)
	}
	if doc.DockerData.Size == nil || *doc.DockerData.Size != 1536 || doc.DockerData.Mark != "reported only" {
		t.Errorf("dockerData = %+v, want size 1536 marked reported only", doc.DockerData)
	}
}

func TestDockerDataUnreadHasNoSizeAndTheScanStillReportsIt(t *testing.T) {
	result := scan.Result{DockerData: domain.Unread("docker is not running")}

	var table strings.Builder
	if err := report.Scan(&table, result, report.Table); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(table.String(), "\n\nDocker data  docker is not running\n") {
		t.Fatalf("table is\n%s\nwant the unread line", table.String())
	}
	if strings.Contains(table.String(), "reported only") {
		t.Fatalf("table is\n%s\nwant no reported-only mark without a size", table.String())
	}

	var out strings.Builder
	if err := report.Scan(&out, result, report.JSON); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		DockerData struct {
			Size *int64
			Mark string
		}
	}
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.DockerData.Size != nil || doc.DockerData.Mark != "docker is not running" {
		t.Errorf("dockerData = %+v, want no size and the reason", doc.DockerData)
	}
}

func TestAZeroDockerDataSizeIsReported(t *testing.T) {
	result := scan.Result{DockerData: domain.Reported(0)}

	var table strings.Builder
	if err := report.Scan(&table, result, report.Table); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(table.String(), "\n\nDocker data  0 B  reported only\n") {
		t.Fatalf("table is\n%s\nwant a zero size marked reported only", table.String())
	}

	var out strings.Builder
	if err := report.Scan(&out, result, report.JSON); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		DockerData struct {
			Size *int64
			Mark string
		}
	}
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.DockerData.Size == nil || *doc.DockerData.Size != 0 || doc.DockerData.Mark != "reported only" {
		t.Errorf("dockerData = %+v, want size 0 marked reported only", doc.DockerData)
	}
}
