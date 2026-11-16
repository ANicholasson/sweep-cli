// Package report prints what a Scan found.
package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

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
// with its path, Size, Kind and Reason, and every path it could not read.
//
// The Table:
//
//	Roots
//	  /work/projects
//
//	SIZE      KIND          PATH                                 REASON
//	84.0 KiB  Dependencies  /work/projects/web-app/node_modules  npm install puts it back from package.json and the lockfile.
//
//	Not read, so the list may be short and a Size may be low
//	  /work/projects/locked: open /work/projects/locked: permission denied
//
// With no Candidate the table is the line "No Candidates." The last section
// is there only when something was not read.
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
		columns := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(columns, "SIZE\tKIND\tPATH\tREASON")
		for _, c := range result.Candidates {
			_, _ = fmt.Fprintf(columns, "%s\t%s\t%s\t%s\n", human(c.Size), c.Rule.Kind, c.Path, c.Rule.Reason)
		}
		_ = columns.Flush()
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
// bytes. Kind is the Kind's name as GLOSSARY.md writes it.
type scanV1 struct {
	Version    int            `json:"version"`
	Roots      []string       `json:"roots"`
	Candidates []candidateV1  `json:"candidates"`
	Unreadable []unreadableV1 `json:"unreadable"`
}

type candidateV1 struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Kind      string `json:"kind"`
	Ecosystem string `json:"ecosystem"`
	Reason    string `json:"reason"`
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
		})
	}
	for _, u := range result.Unreadable {
		out.Unreadable = append(out.Unreadable, unreadableV1{Path: u.Path, Error: u.Err.Error()})
	}
	return out
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
