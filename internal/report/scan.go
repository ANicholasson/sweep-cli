// Package report prints what a Scan found.
package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// Format is how a report is written. Only Table and JSON exist.
type Format struct{ name string }

var (
	// Table is for a person.
	Table = Format{"table"}
	// JSON is for a script: one document, whose shape is scanV1.
	JSON = Format{"json"}
)

// Scan writes everything a Scan found: the Roots it used, each Candidate
// grouped by Project with its path, Size, Kind, Last used, Stale mark and
// Reason, and every path it could not read.
//
// The Table:
//
//	Roots
//	  /work/projects
//
//	/work/projects/web-app
//	SIZE      KIND          PATH                                 LAST USED             STALE  REASON
//	84.0 KiB  Dependencies  /work/projects/web-app/node_modules  2026-09-25 12:00:00Z  stale  npm install puts it back from package.json and the lockfile.
//
//	Not read, so the list may be short and a Size may be low
//	  /work/projects/locked: open /work/projects/locked: permission denied
//
// With no Candidate the table is the line "No Candidates." The last section
// is there only when something was not read. A Candidate with no Project is
// grouped under "(no Project)".
func Scan(w io.Writer, result scan.Result, format Format) error {
	var out bytes.Buffer
	if format == JSON {
		encoder := json.NewEncoder(&out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(toV1(result)); err != nil {
			return err
		}
	} else {
		table(&out, result)
	}
	_, err := w.Write(out.Bytes())
	return err
}

func table(out *bytes.Buffer, result scan.Result) {
	out.WriteString("Roots\n")
	for _, root := range result.Roots {
		fmt.Fprintf(out, "  %s\n", root)
	}
	out.WriteString("\n")
	if len(result.Candidates) == 0 {
		out.WriteString("No Candidates.\n")
	} else {
		for i, g := range grouped(result.Candidates) {
			if i > 0 {
				out.WriteString("\n")
			}
			fmt.Fprintf(out, "%s\n", g.heading)
			columns := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(columns, "SIZE\tKIND\tPATH\tLAST USED\tSTALE\tREASON")
			for _, c := range g.candidates {
				mark := ""
				if c.Stale {
					mark = "stale"
				}
				_, _ = fmt.Fprintf(columns, "%s\t%s\t%s\t%s\t%s\t%s\n", human(c.Size), c.Rule.Kind, c.Path, when(c.LastUsed), mark, c.Rule.Reason)
			}
			_ = columns.Flush()
		}
	}
	if len(result.Unreadable) > 0 {
		out.WriteString("\nNot read, so the list may be short and a Size may be low\n")
		for _, u := range result.Unreadable {
			fmt.Fprintf(out, "  %s: %v\n", u.Path, u.Err)
		}
	}
}

// scanV1 is the JSON of a Scan. A change that could break a script raises
// its version.
//
// Every list is written as [] when it is empty, never null. Size is in
// bytes. Kind is the Kind's name as GLOSSARY.md writes it. project is the
// Project's path, or "" when the Candidate has none. last_used is UTC.
// stale is true when the Scan marked the Candidate Stale.
type scanV1 struct {
	Version    int            `json:"version"`
	Roots      []string       `json:"roots"`
	Candidates []candidateV1  `json:"candidates"`
	Unreadable []unreadableV1 `json:"unreadable"`
}

type candidateV1 struct {
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	Kind      string    `json:"kind"`
	Ecosystem string    `json:"ecosystem"`
	Reason    string    `json:"reason"`
	Project   string    `json:"project"`
	LastUsed  time.Time `json:"last_used"`
	Stale     bool      `json:"stale"`
}

type unreadableV1 struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

func toV1(result scan.Result) scanV1 {
	out := scanV1{Version: 1, Roots: []string{}, Candidates: []candidateV1{}, Unreadable: []unreadableV1{}}
	for _, root := range result.Roots {
		out.Roots = append(out.Roots, string(root))
	}
	for _, c := range result.Candidates {
		out.Candidates = append(out.Candidates, candidateV1{
			Path:      c.Path,
			Size:      int64(c.Size),
			Kind:      c.Rule.Kind.String(),
			Ecosystem: c.Ecosystem,
			Reason:    string(c.Rule.Reason),
			Project:   c.Project,
			LastUsed:  c.LastUsed.UTC(),
			Stale:     c.Stale,
		})
	}
	for _, u := range result.Unreadable {
		out.Unreadable = append(out.Unreadable, unreadableV1{Path: u.Path, Error: u.Err.Error()})
	}
	return out
}

type projectGroup struct {
	heading    string
	candidates []domain.Candidate
}

// grouped keeps the Scan's order and puts Candidates of one Project together,
// in the order the Scan first met that Project. A Candidate with no Project
// stands in a group of its own, because its Last used is its own.
func grouped(candidates []domain.Candidate) []projectGroup {
	var out []projectGroup
	index := map[string]int{}
	for _, c := range candidates {
		if c.Project == "" {
			out = append(out, projectGroup{heading: "(no Project)", candidates: []domain.Candidate{c}})
			continue
		}
		if i, ok := index[c.Project]; ok {
			out[i].candidates = append(out[i].candidates, c)
			continue
		}
		index[c.Project] = len(out)
		out = append(out, projectGroup{heading: c.Project, candidates: []domain.Candidate{c}})
	}
	return out
}

func when(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05Z")
}

func human(size domain.Size) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	value, unit := float64(size)/1024, 0
	for value >= 1024 && unit < 3 {
		value, unit = value/1024, unit+1
	}
	return fmt.Sprintf("%.1f %s", value, [...]string{"KiB", "MiB", "GiB", "TiB"}[unit])
}
