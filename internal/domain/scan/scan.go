package scan

import (
	"cmp"
	"slices"
	"strings"

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
// Docker data is asked once, for the whole machine, and set on the Result.
// It is not a Candidate. Docker missing or not answering is that value's
// own mark, not an Unreadable path, so a Scan that cannot reach Docker is
// still complete.
//
// It cannot fail. What it could not read is in the Result.
func Scan(files Files, git safety.Git, docker Docker, roots []domain.Root, ecosystems []domain.Ecosystem) Result {
	s := scanner{
		files:  files,
		git:    git,
		rules:  index(ecosystems),
		walked: map[FileID]bool{},
		listed: map[FileID]bool{},
		inside: map[FileID]bool{},
	}
	for _, root := range roots {
		s.enter(string(root), string(root))
	}
	result := Result{Roots: slices.Clone(roots), Unreadable: s.unreadable, DockerData: docker.Data()}
	for _, f := range s.found {
		if !s.inside[f.id] {
			result.Candidates = append(result.Candidates, f.candidate)
		}
	}
	return result
}

type scanner struct {
	files  Files
	git    safety.Git
	rules  rules
	walked map[FileID]bool
	listed map[FileID]bool
	inside map[FileID]bool

	found      []found
	unreadable []Unreadable
}

type found struct {
	id        FileID
	candidate domain.Candidate
}

func (s *scanner) enter(root, path string) {
	if !safety.Beneath(root, path, false) {
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
		if !safety.Beneath(root, e.Path, e.Is == Link) || e.Is != Dir {
			continue
		}
		if r, ok := s.rules.recognise(e, entries); ok {
			s.candidate(root, e, r)
			continue
		}
		s.enter(root, e.Path)
	}
}

func (s *scanner) candidate(root string, dir Entry, r recogniser) {
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
	}})
}

func (s *scanner) size(root, path string, own Space) domain.Size {
	counted := map[FileID]bool{own.File: true}
	total := own.Bytes
	var add func(dir string)
	add = func(dir string) {
		entries, err := s.files.List(dir)
		if err != nil {
			s.unread(dir, err)
			return
		}
		for _, e := range entries {
			if !safety.Beneath(root, e.Path, false) {
				continue
			}
			space, err := s.files.Space(e.Path)
			if err != nil {
				s.unread(e.Path, err)
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
