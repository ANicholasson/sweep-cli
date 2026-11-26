package report_test

import (
	"strings"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/report"
)

func TestTheDryRunNamesTheConfigurationFile(t *testing.T) {
	result := scan.Result{Roots: []domain.Root{"/work"}, Configuration: "/config/sweep/config.toml"}

	var out strings.Builder
	if err := report.DryRun(&out, result, nil, "/work/plan.json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Configuration\n  /config/sweep/config.toml\n") {
		t.Errorf("the report is\n%s\nwant the file that was read", out.String())
	}

	result.Configuration = ""
	out.Reset()
	if err := report.DryRun(&out, result, nil, "/work/plan.json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Configuration\n  none\n") {
		t.Errorf("the report is\n%s\nwant none when no file was read", out.String())
	}
}
