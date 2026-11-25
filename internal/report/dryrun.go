package report

import (
	"bytes"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/plan"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// DryRun writes what a dry run decided. The Roots come first. Each
// Candidate is then printed under "picked: <reason>" or "not picked:
// <reason>", with its Size, Kind and path. The report ends with how many
// Candidates and how much Size are in the Plan, the file the Plan was
// written to, what was left out, and, when something was left out, how to
// add it. A path the Scan could not read is listed last, as a Scan lists it.
//
//	Roots
//	  /work/projects
//
//	picked: Stale
//	SIZE      KIND          PATH
//	12.0 KiB  Dependencies  /work/projects/stale-app/node_modules
//
//	not picked: used recently
//	SIZE      KIND          PATH
//	8.0 KiB   Dependencies  /work/projects/recent-app/node_modules
//
//	1 Candidate in the Plan, 12.0 KiB
//	Written to /work/sweep-plan.json
//	Left out 1 Candidate, 8.0 KiB, used recently
//	To add one, pass --pick and its path.
func DryRun(w io.Writer, result scan.Result, choices []plan.Choice, dest string) error {
	var out bytes.Buffer
	out.WriteString("Roots\n")
	for _, root := range result.Roots {
		fmt.Fprintf(&out, "  %s\n", root)
	}
	out.WriteString("\n")
	if len(choices) == 0 {
		out.WriteString("No Candidates.\n")
	} else {
		first := true
		for _, picked := range []bool{true, false} {
			for _, g := range choiceGroups(choices, picked) {
				if !first {
					out.WriteString("\n")
				}
				first = false
				word := "not picked"
				if picked {
					word = "picked"
				}
				fmt.Fprintf(&out, "%s: %s\n", word, g.reason)
				columns := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
				_, _ = fmt.Fprintln(columns, "SIZE\tKIND\tPATH")
				for _, row := range g.rows {
					_, _ = fmt.Fprintf(columns, "%s\t%s\t%s\n", human(row.Candidate.Size), row.Candidate.Rule.Kind, row.Candidate.Path)
				}
				_ = columns.Flush()
			}
		}
	}
	out.WriteString("\n")
	writeSummary(&out, choices, dest)
	writeUnreadable(&out, result.Unreadable)
	_, err := w.Write(out.Bytes())
	return err
}

type choiceGroup struct {
	reason string
	rows   []plan.Choice
}

func choiceGroups(choices []plan.Choice, picked bool) []choiceGroup {
	var out []choiceGroup
	index := map[string]int{}
	for _, row := range choices {
		if row.Picked != picked {
			continue
		}
		if i, ok := index[row.Reason]; ok {
			out[i].rows = append(out[i].rows, row)
			continue
		}
		index[row.Reason] = len(out)
		out = append(out, choiceGroup{reason: row.Reason, rows: []plan.Choice{row}})
	}
	return out
}

func writeSummary(out *bytes.Buffer, choices []plan.Choice, dest string) {
	var pickedN int
	var pickedSize domain.Size
	type left struct {
		reason string
		n      int
		size   domain.Size
	}
	var omitted []left
	index := map[string]int{}
	for _, row := range choices {
		if row.Picked {
			pickedN++
			pickedSize += row.Candidate.Size
			continue
		}
		if i, ok := index[row.Reason]; ok {
			omitted[i].n++
			omitted[i].size += row.Candidate.Size
			continue
		}
		index[row.Reason] = len(omitted)
		omitted = append(omitted, left{row.Reason, 1, row.Candidate.Size})
	}
	fmt.Fprintf(out, "%s in the Plan, %s\n", counted(pickedN, "Candidate", "Candidates"), human(pickedSize))
	fmt.Fprintf(out, "Written to %s\n", dest)
	if len(omitted) == 0 {
		out.WriteString("Left out nothing.\n")
		return
	}
	for _, b := range omitted {
		fmt.Fprintf(out, "Left out %s, %s, %s\n", counted(b.n, "Candidate", "Candidates"), human(b.size), b.reason)
	}
	out.WriteString("To add one, pass --pick and its path.\n")
}

func counted(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
