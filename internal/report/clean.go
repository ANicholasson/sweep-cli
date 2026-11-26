package report

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/clean"
)

// Clean writes what a Clean did. Each Candidate that was moved is listed
// under "Moved to trash", each Skip under "Skipped" with the Safety rule
// that kept it, and each Candidate that could not be moved under "Failed".
// The summary says the moved space was moved to the trash. It never says
// the space was reclaimed.
func Clean(w io.Writer, result clean.Result) error {
	var out bytes.Buffer
	writeItems(&out, "Moved to trash", result.Trashed, func(item clean.Item) string {
		return item.Path
	})
	writeSkipped(&out, result.Skipped)
	writeFailed(&out, result.Failed)
	fmt.Fprintf(&out, "Moved to trash: %s\n", amount(result.Trashed))
	fmt.Fprintf(&out, "Skipped %s\n", amount(sizesOfSkips(result.Skipped)))
	fmt.Fprintf(&out, "Failed %s\n", amount(sizesOfFails(result.Failed)))
	_, err := w.Write(out.Bytes())
	return err
}

func writeItems(out *bytes.Buffer, title string, items []clean.Item, path func(clean.Item) string) {
	fmt.Fprintf(out, "%s\n", title)
	if len(items) == 0 {
		out.WriteString("Nothing.\n\n")
		return
	}
	columns := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(columns, "SIZE\tPATH")
	for _, item := range items {
		_, _ = fmt.Fprintf(columns, "%s\t%s\n", human(item.Size), path(item))
	}
	_ = columns.Flush()
	out.WriteString("\n")
}

func writeSkipped(out *bytes.Buffer, skips []clean.Skip) {
	out.WriteString("Skipped\n")
	if len(skips) == 0 {
		out.WriteString("Nothing.\n\n")
		return
	}
	columns := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(columns, "RULE\tSIZE\tPATH")
	for _, skip := range skips {
		_, _ = fmt.Fprintf(columns, "%s\t%s\t%s\n", skip.Rule, human(skip.Size), skip.Path)
	}
	_ = columns.Flush()
	out.WriteString("\n")
}

func writeFailed(out *bytes.Buffer, fails []clean.Failure) {
	out.WriteString("Failed\n")
	if len(fails) == 0 {
		out.WriteString("Nothing.\n\n")
		return
	}
	columns := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(columns, "SIZE\tPATH\tERROR")
	for _, fail := range fails {
		_, _ = fmt.Fprintf(columns, "%s\t%s\t%s\n", human(fail.Size), fail.Path, oneLine(fail.Err))
	}
	_ = columns.Flush()
	out.WriteString("\n")
}

func oneLine(err error) string {
	if err == nil {
		return ""
	}
	return strings.ReplaceAll(err.Error(), "\n", " ")
}

func amount(items []clean.Item) string {
	if len(items) == 0 {
		return "nothing"
	}
	var sum domain.Size
	for _, item := range items {
		sum += item.Size
	}
	return counted(len(items), "Candidate", "Candidates") + ", " + human(sum)
}

func sizesOfSkips(skips []clean.Skip) []clean.Item {
	out := make([]clean.Item, len(skips))
	for i, skip := range skips {
		out[i] = skip.Item
	}
	return out
}

func sizesOfFails(fails []clean.Failure) []clean.Item {
	out := make([]clean.Item, len(fails))
	for i, fail := range fails {
		out[i] = fail.Item
	}
	return out
}
