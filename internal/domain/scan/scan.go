package scan

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/safety"
)

// Result is what a Scan found.
type Result struct {
	// Roots are the Roots the Scan looked beneath, in the order given.
	Roots []domain.Root
	// Candidates are in the order the Scan met them: Root by Root, and in
	// each directory in order of name. Two Scans of an unchanged tree with
	// the same Roots list the same Candidates in the same order.
	Candidates []domain.Candidate
	// Unreadable is every path the Scan could not read. A Scan with any is
	// incomplete: a Candidate beneath one is missing, and the Size of a
	// Candidate that holds one leaves it out. A path read through a Root
	// inside a Candidate, and again through the Root that holds the
	// Candidate, is in it twice.
	Unreadable []Unreadable
	// DockerData is the space Docker images and build cache hold, or why
	// it could not be read. It is never a Candidate.
	DockerData domain.DockerData
	// Configuration is the configuration file that was read, or empty when
	// there was none. A Scan leaves it empty.
	Configuration string
}

// Unreadable is a path the Scan could not read, and why.
type Unreadable struct {
	Path string
	Err  error
}

// Docker reports Docker data. It only reads: it has no method that removes
// or changes anything.
type Docker interface {
	Data() domain.DockerData
}

// Settings is the Stale threshold and the Ignore list for one Scan.
// StaleAfter is how old Last used must be before a Candidate is Stale.
// Zero means a Candidate is Stale when its Last used is before now.
// Each Ignore path, and everything beneath it, is left out of the Scan.
type Settings struct {
	StaleAfter time.Duration
	Ignore     []string
}

// Scan looks beneath each Root for the directories that the InProject Rules
// of the Ecosystems recognise, and measures the Size of each. It leaves
// Shared cache Rules alone. A directory the Git rule keeps is not a
// Candidate.
//
// It never enters a Candidate, a directory the Git rule keeps, or a Link,
// and it never enters or reports a path that is not beneath the Root it is
// walking. Nothing inside a Candidate is listed, and no Candidate is listed
// twice, whatever order the Roots come in and however they overlap. When two Rules
// recognise one directory, it is listed under the Rule of the Ecosystem
// whose Name sorts first, and under the earlier of that Ecosystem's Rules,
// whatever order the Ecosystems come in.
//
// Last used is the newest modification among a Project's own files, outside
// its Candidates. A Candidate is left out by its file identity, so another
// spelling of the same directory does not count. A Candidate with no Project
// takes the newest modification inside it. That time is a modification time,
// never an access time. A Link is not followed. A Candidate is not Stale
// when a directory in its Project could not be read, or a file's
// modification time could not be read, because recent work could be in
// what was not seen. Otherwise it is Stale when its Last used is older
// than StaleAfter before now.
//
// A path on the Ignore list, and everything beneath it, is left out of
// the report. It is not a Candidate, and a path beneath it that could not
// be read is not reported. Last used still counts the files there, so an
// Ignore does not by itself change whether a Candidate is Stale. A path
// that only shares a prefix with an ignored path is kept.
//
// Docker data is asked once, for the whole machine, and set on the Result.
// It is not a Candidate. Docker missing or not answering is that value's
// own mark, not an Unreadable path, so a Scan that cannot reach Docker is
// still complete.
//
// It cannot fail. What it could not read is in the Result.
func Scan(files Files, git safety.Git, docker Docker, roots []domain.Root, ecosystems []domain.Ecosystem, now time.Time, settings Settings) Result {
	s := scanner{
		files:  files,
		git:    git,
		rules:  index(ecosystems),
		walked: map[FileID]bool{},
		listed: map[FileID]bool{},
		inside: map[FileID]bool{},
		ignore: settings.Ignore,
	}
	for _, root := range roots {
		s.enter(string(root), string(root))
	}
	var candidates []domain.Candidate
	skip := map[FileID]bool{}
	for _, f := range s.found {
		if s.inside[f.id] {
			continue
		}
		candidates = append(candidates, f.candidate)
		skip[f.id] = true
	}
	s.stamp(candidates, skip, now, settings.StaleAfter)
	return Result{Roots: slices.Clone(roots), Candidates: candidates, Unreadable: s.unreadable, DockerData: docker.Data()}
}

type scanner struct {
	files  Files
	git    safety.Git
	rules  rules
	walked map[FileID]bool
	listed map[FileID]bool
	inside map[FileID]bool
	ignore []string

	found      []found
	unreadable []Unreadable
}

type found struct {
	id        FileID
	candidate domain.Candidate
}

func (s *scanner) enter(root, path string) {
	if s.ignored(path) || !safety.Beneath(root, path, false) {
		return
	}
	space, err := s.files.Space(path)
	if err != nil {
		s.unread(path, err)
		return
	}
	if s.walked[space.File] {
		return
	}
	s.walked[space.File] = true
	entries, err := s.files.List(path)
	if err != nil {
		s.unread(path, err)
		return
	}
	for _, e := range entries {
		if s.ignored(e.Path) || !safety.Beneath(root, e.Path, e.Is == Link) || e.Is != Dir {
			continue
		}
		if r, ok := s.rules.recognise(e, entries); ok {
			s.candidate(root, e, r, path)
			continue
		}
		s.enter(root, e.Path)
	}
}

func (s *scanner) candidate(root string, dir Entry, r recogniser, project string) {
	space, err := s.files.Space(dir.Path)
	if err != nil {
		s.unread(dir.Path, err)
		return
	}
	if s.listed[space.File] {
		return
	}
	s.listed[space.File] = true
	if safety.Keeps(s.git, dir.Path) {
		return
	}
	s.found = append(s.found, found{space.File, domain.Candidate{
		Path:      dir.Path,
		Size:      s.size(root, dir.Path, space),
		Ecosystem: r.ecosystem,
		Rule:      r.rule,
		Project:   project,
	}})
}

// stamp sets Last used and Stale. Candidates of one Project share one time.
// A Candidate with no Project is measured on its own, from the files inside it.
func (s *scanner) stamp(candidates []domain.Candidate, skip map[FileID]bool, now time.Time, staleAfter time.Duration) {
	type age struct {
		at      time.Time
		blocked bool
	}
	known := map[string]age{}
	staleBefore := now.Add(-staleAfter)
	for i := range candidates {
		c := &candidates[i]
		var used time.Time
		var blocked bool
		if c.Project == "" {
			used, blocked = s.newest(c.Path, nil)
		} else if got, ok := known[c.Project]; ok {
			used, blocked = got.at, got.blocked
		} else {
			used, blocked = s.newest(c.Project, skip)
			known[c.Project] = age{used, blocked}
		}
		c.LastUsed = used
		c.Stale = !blocked && used.Before(staleBefore)
	}
}

// newest is the newest modification of the regular files at top and beneath
// it. It never enters or reads a path that is not beneath top. A directory whose file identity is in skip is a Candidate and is not
// entered, whichever path reaches it. A Link is not followed. The bool is
// true when a directory could not be read, or a file's modification time
// could not be read, so recent work could be in what was not seen.
// A path discovery already could not read is not reported again.
func (s *scanner) newest(top string, skip map[FileID]bool) (time.Time, bool) {
	var newest time.Time
	var blocked bool
	var walk func(string)
	walk = func(dir string) {
		entries, err := s.files.List(dir)
		if err != nil {
			s.missed(dir, err)
			blocked = true
			return
		}
		for _, e := range entries {
			if !safety.Beneath(top, e.Path, e.Is == Link) {
				continue
			}
			switch e.Is {
			case Dir:
				space, err := s.files.Space(e.Path)
				if err != nil {
					s.missed(e.Path, err)
					blocked = true
					continue
				}
				if skip[space.File] {
					continue
				}
				walk(e.Path)
			case File:
				mod, err := s.files.Modified(e.Path)
				if err != nil {
					s.missed(e.Path, err)
					blocked = true
					continue
				}
				if mod.After(newest) {
					newest = mod
				}
			}
		}
	}
	walk(top)
	return newest, blocked
}

// missed records a path the Scan could not read. An ignored path is left
// out of that report. The caller still treats the failure as unseen, so a
// Candidate is not Stale when recent work could be in what was not read.
func (s *scanner) missed(path string, err error) {
	if !s.ignored(path) {
		s.noteUnread(path, err)
	}
}

func (s *scanner) ignored(path string) bool {
	for _, root := range s.ignore {
		if safety.Beneath(root, path, false) {
			return true
		}
	}
	return false
}

// noteUnread records a path the Last used walk could not read, unless that
// path is already recorded. Discovery can meet one path twice, through a
// Root inside a Candidate and again through the Root that holds it, and
// those two stay. This walk is not a second Root.
func (s *scanner) noteUnread(path string, err error) {
	for _, u := range s.unreadable {
		if u.Path == path {
			return
		}
	}
	s.unread(path, err)
}

func (s *scanner) size(root, path string, own Space) domain.Size {
	counted := map[FileID]bool{own.File: true}
	total := own.Bytes
	var add func(dir string)
	add = func(dir string) {
		entries, err := s.files.List(dir)
		if err != nil {
			s.missed(dir, err)
			return
		}
		for _, e := range entries {
			if !safety.Beneath(root, e.Path, false) {
				continue
			}
			space, err := s.files.Space(e.Path)
			if err != nil {
				s.missed(e.Path, err)
				continue
			}
			if counted[space.File] {
				continue
			}
			counted[space.File] = true
			total += space.Bytes
			if e.Is == Dir {
				s.inside[space.File] = true
				add(e.Path)
			}
		}
	}
	add(path)
	return domain.Size(total)
}

func (s *scanner) unread(path string, err error) {
	s.unreadable = append(s.unreadable, Unreadable{Path: path, Err: err})
}

type rules map[string][]recogniser

type recogniser struct {
	manifest  string
	ecosystem string
	rule      domain.Rule
}

func index(ecosystems []domain.Ecosystem) rules {
	byName := slices.Clone(ecosystems)
	slices.SortFunc(byName, func(a, b domain.Ecosystem) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Name, b.Name))
	})
	found := rules{}
	for _, e := range byName {
		for _, r := range e.Rules {
			if w, ok := r.Where.(domain.InProject); ok {
				found[w.Candidate] = append(found[w.Candidate], recogniser{w.Manifest, e.Name, r})
			}
		}
	}
	return found
}

func (rs rules) recognise(dir Entry, beside []Entry) (recogniser, bool) {
	for _, r := range rs[dir.Name] {
		if slices.ContainsFunc(beside, func(e Entry) bool { return e.Name == r.manifest && e.Is == File }) {
			return r, true
		}
	}
	return recogniser{}, false
}
