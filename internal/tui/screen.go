package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/plan"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

const keys = "j/k move    t tick    u untick    q quit"

type Screen struct {
	lines  []line
	choose []int
	at     int
	docker domain.DockerData
	unread []scan.Unreadable
}

type kind int

const (
	heading kind = iota
	candidate
	docker
)

type line struct {
	kind      kind
	heading   string
	candidate domain.Candidate
	picked    bool
}

func New(result scan.Result) Screen {
	var lines []line
	var choose []int
	if len(result.Candidates) == 0 {
		lines = append(lines, line{kind: heading, heading: "No Candidates."})
	}
	for _, g := range groups(result.Candidates) {
		lines = append(lines, line{kind: heading, heading: g.heading})
		for _, c := range g.candidates {
			choose = append(choose, len(lines))
			lines = append(lines, line{kind: candidate, candidate: c, picked: plan.StartsPicked(c)})
		}
	}
	choose = append(choose, len(lines))
	lines = append(lines, line{kind: docker})
	return Screen{lines: lines, choose: choose, docker: result.DockerData, unread: result.Unreadable}
}

func (s *Screen) Press(key []byte) bool {
	switch string(key) {
	case "q", "\x03":
		return true
	case "j", "\x1b[B", "\x1bOB":
		s.move(1)
	case "k", "\x1b[A", "\x1bOA":
		s.move(-1)
	case "t":
		s.pick(true)
	case "u":
		s.pick(false)
	}
	return false
}

func (s Screen) Text() string {
	var out strings.Builder
	for i, g := range s.sections() {
		if i > 0 {
			out.WriteString("\n")
		}
		fmt.Fprintf(&out, "%s\n", g.heading)
		columns := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
		for _, row := range g.rows {
			_, _ = fmt.Fprintf(columns, "%s[%s]\t%s\t%s\t%s\t%s\n",
				s.mark(row.index), box(row.picked), human(row.candidate.Size), row.candidate.Rule.Kind, when(row.candidate.LastUsed), shownPath(row.candidate))
		}
		_ = columns.Flush()
	}
	out.WriteString("\n")
	_, _ = fmt.Fprintf(&out, "%s%s\n", s.mark(s.dockerIndex()), dockerText(s.docker))
	_, _ = fmt.Fprintf(&out, "\nReason\n%s\n", s.reason())
	n, size := s.picked()
	_, _ = fmt.Fprintf(&out, "\n%s, %s picked\n%s\n", counted(n), human(size), keys)
	writeUnread(&out, s.unread)
	return out.String()
}

func (s Screen) Page(width, height int) string {
	if width < 1 || height < 1 {
		return s.Text()
	}
	lines := strings.Split(strings.TrimRight(s.Text(), "\n"), "\n")
	footerAt := len(lines)
	for i, line := range lines {
		if strings.Contains(line, "Docker data") {
			footerAt = i
			if i > 0 && lines[i-1] == "" {
				footerAt = i - 1
			}
			break
		}
	}
	body, footer := lines[:footerAt], clipLines(lines[footerAt:], width)
	hi := -1
	for i, line := range body {
		if strings.HasPrefix(line, "> ") {
			hi = i
			break
		}
	}
	if len(footer) >= height {
		return strings.Join(footer[len(footer)-height:], "\n") + "\n"
	}
	window := clipLines(windowAround(body, hi, height-len(footer)), width)
	return strings.Join(append(window, footer...), "\n") + "\n"
}

func windowAround(lines []string, hi, budget int) []string {
	if len(lines) == 0 || budget <= 0 {
		return nil
	}
	if len(lines) <= budget {
		return lines
	}
	if hi < 0 || hi >= len(lines) {
		hi = len(lines) - 1
	}
	start := hi - (budget - 1)
	if start < 0 {
		start = 0
	}
	end := start + budget
	if end > len(lines) {
		end = len(lines)
		start = end - budget
	}
	return lines[start:end]
}

func clipLines(lines []string, width int) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = clip(line, width)
	}
	return out
}

func clip(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 3 {
		return string(r[:width])
	}
	return string(r[:width-3]) + "..."
}

func (s *Screen) move(delta int) {
	next := s.at + delta
	if next < 0 || next >= len(s.choose) {
		return
	}
	s.at = next
}

func (s *Screen) pick(picked bool) {
	row := s.lines[s.choose[s.at]]
	if row.kind != candidate {
		return
	}
	s.lines[s.choose[s.at]].picked = picked
}

func (s Screen) mark(index int) string {
	if s.choose[s.at] == index {
		return "> "
	}
	return "  "
}

func (s Screen) dockerIndex() int {
	return s.choose[len(s.choose)-1]
}

func (s Screen) reason() string {
	row := s.lines[s.choose[s.at]]
	if row.kind == docker {
		return s.docker.Mark()
	}
	return string(row.candidate.Rule.Reason)
}

func (s Screen) picked() (int, domain.Size) {
	var n int
	var size domain.Size
	for _, row := range s.lines {
		if row.kind == candidate && row.picked {
			n++
			size += row.candidate.Size
		}
	}
	return n, size
}

type section struct {
	heading string
	rows    []shown
}

type shown struct {
	index     int
	candidate domain.Candidate
	picked    bool
}

func (s Screen) sections() []section {
	var out []section
	for i, row := range s.lines {
		switch row.kind {
		case heading:
			out = append(out, section{heading: row.heading})
		case candidate:
			n := len(out) - 1
			out[n].rows = append(out[n].rows, shown{index: i, candidate: row.candidate, picked: row.picked})
		}
	}
	return out
}

type group struct {
	heading    string
	candidates []domain.Candidate
}

func groups(candidates []domain.Candidate) []group {
	var out []group
	index := map[string]int{}
	shared := -1
	for _, c := range candidates {
		if c.Rule.SharedCache() {
			if shared < 0 {
				out = append(out, group{heading: "Shared caches", candidates: []domain.Candidate{c}})
				shared = len(out) - 1
				continue
			}
			out[shared].candidates = append(out[shared].candidates, c)
			continue
		}
		if c.Project == "" {
			out = append(out, group{heading: "(no Project)", candidates: []domain.Candidate{c}})
			continue
		}
		if i, ok := index[c.Project]; ok {
			out[i].candidates = append(out[i].candidates, c)
			continue
		}
		index[c.Project] = len(out)
		out = append(out, group{heading: c.Project, candidates: []domain.Candidate{c}})
	}
	return out
}

func shownPath(c domain.Candidate) string {
	if c.Project != "" {
		rel, err := filepath.Rel(c.Project, c.Path)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return c.Path
}

func box(picked bool) string {
	if picked {
		return "x"
	}
	return " "
}

func dockerText(data domain.DockerData) string {
	if size, ok := data.Size(); ok {
		return fmt.Sprintf("Docker data  %s  %s", human(size), data.Mark())
	}
	return fmt.Sprintf("Docker data  %s", data.Mark())
}

func writeUnread(out *strings.Builder, unread []scan.Unreadable) {
	if len(unread) == 0 {
		return
	}
	out.WriteString("\nNot read, so the list may be short and a Size may be low\n")
	for _, u := range unread {
		fmt.Fprintf(out, "  %s: %v\n", u.Path, u.Err)
	}
}

func counted(n int) string {
	if n == 1 {
		return "1 Candidate"
	}
	return fmt.Sprintf("%d Candidates", n)
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
