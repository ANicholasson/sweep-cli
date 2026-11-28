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

const (
	listKeys    = "j/k move    t tick    u untick    enter review    q quit"
	reviewKeys  = "j/k move    y confirm    n back"
	outcomeKeys = "j/k move    q quit"
)

// Effect is what one key asks the composition root to do. Stay means the
// screen only changed its drawing. Quit leaves without a Clean. Review
// means the review opened, so the Plan can be recorded before anything
// moves. Confirm asks for the Clean of that Plan, and the screen waits.
type Effect uint8

const (
	Stay Effect = iota
	Quit
	Review
	Confirm
)

type mode uint8

const (
	modeList mode = iota
	modeReview
	modeOutcome
)

type Screen struct {
	lines   []line
	choose  []int
	at      int
	docker  domain.DockerData
	unread  []scan.Unreadable
	order   []domain.Candidate
	picked  map[string]bool
	mode    mode
	outcome string
	focus   int
}

type kind int

const (
	heading kind = iota
	candidate
	docker
	gap
)

type line struct {
	kind      kind
	heading   string
	candidate domain.Candidate
}

func New(result scan.Result) Screen {
	picked := map[string]bool{}
	for _, c := range result.Candidates {
		if plan.StartsPicked(c) {
			picked[c.Path] = true
		}
	}
	var lines []line
	var choose []int
	if len(result.Candidates) == 0 {
		lines = append(lines, line{kind: heading, heading: "No Candidates."})
	}
	for _, g := range groups(result.Candidates) {
		lines = append(lines, line{kind: heading, heading: g.heading})
		for _, c := range g.candidates {
			choose = append(choose, len(lines))
			lines = append(lines, line{kind: candidate, candidate: c})
		}
	}
	choose = append(choose, len(lines))
	lines = append(lines, line{kind: docker})
	return Screen{
		lines:  lines,
		choose: choose,
		docker: result.DockerData,
		unread: result.Unreadable,
		order:  append([]domain.Candidate(nil), result.Candidates...),
		picked: picked,
	}
}

// Picked is the ticked Candidates in Scan order. That is the order a Plan
// records them.
func (s Screen) Picked() []domain.Candidate {
	var out []domain.Candidate
	for _, c := range s.order {
		if s.picked[c.Path] {
			out = append(out, c)
		}
	}
	return out
}

func (s *Screen) ShowOutcome(text string) {
	s.mode = modeOutcome
	s.outcome = text
	s.focus = 0
}

// Decoder turns terminal bytes into keys. An escape sequence that arrives
// in pieces is kept until a later Push completes it.
type Decoder struct {
	pending []byte
}

func (d *Decoder) Push(chunk []byte) [][]byte {
	if len(chunk) == 0 {
		return nil
	}
	d.pending = append(d.pending, chunk...)
	var keys [][]byte
	for {
		key, rest, ok := takeKey(d.pending)
		if !ok {
			return keys
		}
		d.pending = rest
		keys = append(keys, key)
	}
}

func takeKey(b []byte) (key, rest []byte, ok bool) {
	if len(b) == 0 {
		return nil, nil, false
	}
	if b[0] != 0x1b {
		return []byte{b[0]}, b[1:], true
	}
	if len(b) == 1 {
		return nil, b, false
	}
	if b[1] == '[' || b[1] == 'O' {
		if len(b) < 3 {
			return nil, b, false
		}
		return append([]byte(nil), b[:3]...), b[3:], true
	}
	return []byte{b[0]}, b[1:], true
}

func (s *Screen) Press(key []byte) Effect {
	switch s.mode {
	case modeReview:
		return s.pressReview(key)
	case modeOutcome:
		return s.pressOutcome(key)
	default:
		return s.pressList(key)
	}
}

func (s *Screen) pressList(key []byte) Effect {
	switch string(key) {
	case "q", "\x03":
		return Quit
	case "\r", "\n":
		s.mode = modeReview
		s.focus = 0
		return Review
	case "j", "\x1b[B", "\x1bOB":
		s.move(1)
	case "k", "\x1b[A", "\x1bOA":
		s.move(-1)
	case "t":
		s.pick(true)
	case "u":
		s.pick(false)
	}
	return Stay
}

func (s *Screen) pressReview(key []byte) Effect {
	switch string(key) {
	case "y":
		return Confirm
	case "n":
		s.mode = modeList
	case "q", "\x03":
		return Quit
	case "j", "\x1b[B", "\x1bOB":
		s.moveFocus(1, len(s.reviewBody()))
	case "k", "\x1b[A", "\x1bOA":
		s.moveFocus(-1, len(s.reviewBody()))
	}
	return Stay
}

func (s *Screen) pressOutcome(key []byte) Effect {
	switch string(key) {
	case "q", "\x03":
		return Quit
	case "j", "\x1b[B", "\x1bOB":
		s.moveFocus(1, len(s.outcomeBody()))
	case "k", "\x1b[A", "\x1bOA":
		s.moveFocus(-1, len(s.outcomeBody()))
	}
	return Stay
}

func (s *Screen) moveFocus(delta, n int) {
	if n < 1 {
		s.focus = 0
		return
	}
	next := s.focus + delta
	if next < 0 {
		next = 0
	}
	if next >= n {
		next = n - 1
	}
	s.focus = next
}

type drawn struct {
	text string
	kind kind
	keep bool
}

func (s Screen) Text() string {
	return joinDrawn(s.view())
}

func (s Screen) view() []drawn {
	switch s.mode {
	case modeReview:
		return s.reviewLines()
	case modeOutcome:
		return s.outcomeLines()
	default:
		return append(append(s.bodyLines(), s.chromeLines()...), s.diagLines()...)
	}
}

const notRead = "Not read, so the list may be short and a Size may be low"

func (s Screen) Page(width, height int) string {
	if s.mode != modeList {
		body, foot := s.parts()
		return pageAround(body, foot, s.focus, width, height)
	}
	if width < 1 || height < 1 {
		return s.Text()
	}
	body := s.bodyLines()
	footer := s.chromeLines()
	diag := s.diagLines()
	hi := -1
	for i, line := range body {
		if line.keep {
			hi = i
			break
		}
	}
	// The list, including the heading of the highlighted Candidate, comes
	// before the unread paths. Those paths take only the rows left over.
	minBody := 0
	if hi >= 0 {
		minBody = spanWithHeading(body, hi)
		if minBody > height {
			minBody = height
		}
	}
	footer = fitFooter(footer, height-minBody)
	remaining := height - len(footer)
	if remaining < 0 {
		remaining = 0
	}
	bodyBudget := len(body)
	if bodyBudget > remaining {
		bodyBudget = remaining
	}
	diag = fitDiag(diag, remaining-bodyBudget)
	window := clipLines(texts(windowAround(body, hi, bodyBudget)), width)
	parts := append(append(window, clipLines(texts(footer), width)...), clipLines(texts(diag), width)...)
	return strings.Join(parts, "\n") + "\n"
}

func (s Screen) bodyLines() []drawn {
	var lines []drawn
	for i, g := range s.sections() {
		if i > 0 {
			lines = append(lines, drawn{kind: gap})
		}
		lines = append(lines, drawn{text: g.heading, kind: heading})
		var buf strings.Builder
		columns := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
		var indexes []int
		for _, row := range g.rows {
			indexes = append(indexes, row.index)
			_, _ = fmt.Fprintf(columns, "%s[%s]\t%s\t%s\t%s\t%s\n",
				s.mark(row.index), box(row.picked), human(row.candidate.Size), row.candidate.Rule.Kind, when(row.candidate.LastUsed), shownPath(row.candidate))
		}
		_ = columns.Flush()
		rendered := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		if buf.Len() == 0 {
			rendered = nil
		}
		for j, text := range rendered {
			line := drawn{text: text, kind: candidate}
			if j < len(indexes) && s.choose[s.at] == indexes[j] {
				line.keep = true
			}
			lines = append(lines, line)
		}
	}
	return lines
}

func (s Screen) chromeLines() []drawn {
	n, size := s.pickedCount()
	onDocker := s.lines[s.choose[s.at]].kind == docker
	return []drawn{
		{kind: gap},
		{text: s.mark(s.dockerIndex()) + dockerText(s.docker), kind: docker, keep: onDocker},
		{kind: gap},
		{text: "Reason"},
		{text: s.reason()},
		{kind: gap},
		{text: fmt.Sprintf("%s, %s picked", counted(n), human(size)), keep: true},
		{text: listKeys},
	}
}

func (s Screen) parts() (body, foot []drawn) {
	if s.mode == modeOutcome {
		return s.outcomeBody(), s.outcomeFoot()
	}
	return s.reviewBody(), s.reviewFoot()
}

func (s Screen) reviewLines() []drawn {
	return append(s.reviewBody(), s.reviewFoot()...)
}

func (s Screen) reviewBody() []drawn {
	rows := s.Picked()
	lines := []drawn{{text: "Will be moved to trash"}}
	if len(rows) == 0 {
		return append(lines, drawn{text: "Nothing."})
	}
	var buf strings.Builder
	columns := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(columns, "SIZE\tPATH")
	for _, c := range rows {
		_, _ = fmt.Fprintf(columns, "%s\t%s\n", human(c.Size), c.Path)
	}
	_ = columns.Flush()
	for _, text := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		lines = append(lines, drawn{text: text})
	}
	return lines
}

func (s Screen) reviewFoot() []drawn {
	n, size := s.pickedCount()
	return []drawn{
		{kind: gap},
		{text: fmt.Sprintf("%s, %s will be moved to trash", counted(n), human(size)), keep: true},
		{text: reviewKeys},
	}
}

func (s Screen) outcomeLines() []drawn {
	return append(s.outcomeBody(), s.outcomeFoot()...)
}

func (s Screen) outcomeBody() []drawn {
	text := strings.TrimRight(s.outcome, "\n")
	if text == "" {
		return []drawn{{text: "Nothing."}}
	}
	var lines []drawn
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, drawn{text: line})
	}
	return lines
}

func (s Screen) outcomeFoot() []drawn {
	return []drawn{{kind: gap}, {text: outcomeKeys, keep: true}}
}

func pageAround(body, footer []drawn, focus, width, height int) string {
	if width < 1 || height < 1 {
		return joinDrawn(append(append([]drawn{}, body...), footer...))
	}
	foot := footer
	if len(foot) >= height && height >= 2 {
		foot = foot[len(foot)-1:]
	}
	budget := height - len(foot)
	if budget < 0 {
		budget = 0
	}
	body = markFocus(body, focus)
	window := clipLines(texts(windowFrom(body, focus, budget)), width)
	parts := append(window, clipLines(texts(foot), width)...)
	return strings.Join(parts, "\n") + "\n"
}

func markFocus(lines []drawn, focus int) []drawn {
	if len(lines) == 0 {
		return lines
	}
	if focus < 0 || focus >= len(lines) {
		focus = 0
	}
	out := make([]drawn, len(lines))
	copy(out, lines)
	for i := range out {
		if i == focus {
			out[i].text = "> " + out[i].text
			continue
		}
		out[i].text = "  " + out[i].text
	}
	return out
}

func windowFrom(lines []drawn, focus, budget int) []drawn {
	if len(lines) == 0 || budget <= 0 {
		return nil
	}
	if len(lines) <= budget {
		return lines
	}
	if focus < 0 {
		focus = 0
	}
	if focus > len(lines)-budget {
		focus = len(lines) - budget
	}
	return lines[focus : focus+budget]
}

func (s Screen) diagLines() []drawn {
	if len(s.unread) == 0 {
		return nil
	}
	lines := []drawn{{kind: gap}, {text: notRead}}
	for _, u := range s.unread {
		lines = append(lines, drawn{text: fmt.Sprintf("  %s: %v", u.Path, u.Err)})
	}
	return lines
}

func spanWithHeading(body []drawn, hi int) int {
	n := 1
	if hi > 0 && body[hi-1].kind == heading {
		n++
	}
	return n
}

func fitFooter(footer []drawn, limit int) []drawn {
	if limit <= 0 || len(footer) == 0 {
		return nil
	}
	if len(footer) <= limit {
		return footer
	}
	var must []int
	for i, line := range footer {
		if line.keep {
			must = append(must, i)
		}
	}
	if len(must) > limit {
		must = must[len(must)-limit:]
	}
	keep := map[int]bool{}
	for _, i := range must {
		keep[i] = true
	}
	for i := len(footer) - 1; i >= 0 && len(keep) < limit; i-- {
		keep[i] = true
	}
	var out []drawn
	for i, line := range footer {
		if keep[i] {
			out = append(out, line)
		}
	}
	return out
}

func fitDiag(diag []drawn, room int) []drawn {
	if room <= 0 || len(diag) == 0 {
		return nil
	}
	if len(diag) <= room {
		return diag
	}
	var paths []string
	for _, line := range diag {
		if strings.HasPrefix(line.text, "  ") {
			paths = append(paths, line.text)
		}
	}
	if room == 1 {
		return []drawn{{text: fmt.Sprintf("%d paths were not read", len(paths))}}
	}
	out := []drawn{{text: notRead}}
	slots := room - 1
	if len(paths) <= slots {
		for _, path := range paths {
			out = append(out, drawn{text: path})
		}
		return out
	}
	if slots == 1 {
		return append(out, drawn{text: fmt.Sprintf("  and %d more", len(paths))})
	}
	for _, path := range paths[:slots-1] {
		out = append(out, drawn{text: path})
	}
	return append(out, drawn{text: fmt.Sprintf("  and %d more", len(paths)-(slots-1))})
}

func windowAround(lines []drawn, hi, budget int) []drawn {
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

func texts(lines []drawn) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = line.text
	}
	return out
}

func joinDrawn(lines []drawn) string {
	return strings.Join(texts(lines), "\n") + "\n"
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

func (s *Screen) pick(on bool) {
	row := s.lines[s.choose[s.at]]
	if row.kind != candidate {
		return
	}
	if on {
		s.picked[row.candidate.Path] = true
		return
	}
	delete(s.picked, row.candidate.Path)
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

func (s Screen) pickedCount() (int, domain.Size) {
	var n int
	var size domain.Size
	for _, c := range s.order {
		if s.picked[c.Path] {
			n++
			size += c.Size
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
			out[n].rows = append(out[n].rows, shown{index: i, candidate: row.candidate, picked: s.picked[row.candidate.Path]})
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
