package scan

import (
	"cmp"
	"errors"
	"io/fs"
	"path"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/safety"
)

// Result is what a Scan found.
type Result struct {
	// Roots are the Roots the Scan looked beneath. The ones the caller gave
	// come first, in the order given, then the Shared caches sweep added.
	Roots []domain.Root
	// Added are the Shared cache Roots sweep added, in the order they
	// appear in Roots. A Root the caller gave is never here.
	Added []domain.Root
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
	// StaleAfter is the threshold this Scan used.
	StaleAfter time.Duration
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

// Settings is what one Scan was asked to do beyond the Roots it was given.
// StaleAfter is how old Last used must be before a Candidate is Stale.
// Zero means a Candidate is Stale when its Last used is before now.
// Each Ignore path, and everything beneath it, is left out of the Scan.
// Home is the user's home directory. Shared caches are added only inside
// it, and an empty Home adds none. SharedCaches adds each known Shared
// cache as a Root. Off, a Scan looks only beneath the Roots it was given.
type Settings struct {
	StaleAfter   time.Duration
	Ignore       []string
	Home         string
	SharedCaches bool
}

// Scan looks beneath each Root for the directories that the InProject Rules
// of the Ecosystems recognise, and measures the Size of each. A directory
// the Git rule keeps is not a Candidate.
//
// When SharedCaches is set, each InSharedCache Rule adds its directory on
// this operating system as a Root, after the Roots the caller gave, and
// that directory is one Candidate. A location that is not inside Home is
// not added, which the Scan checks when it would add the Root. A missing
// directory is not added. With Split, each entry directly inside the
// directory is a Candidate and the directory itself is not. A Shared cache
// is not walked for InProject Rules.
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
		files:         files,
		git:           git,
		rules:         index(ecosystems),
		caches:        map[string]placed{},
		cacheID:       map[FileID]placed{},
		fileCandidate: map[string]bool{},
		walked:        map[FileID]bool{},
		listed:        map[FileID]bool{},
		inside:        map[FileID]bool{},
		ignore:        settings.Ignore,
		formOf:        map[string]string{},
	}
	roots = slices.Clone(roots)
	var added []domain.Root
	if settings.SharedCaches {
		added = s.placeCaches(ecosystems, settings.Home, roots)
		roots = append(roots, added...)
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
	return Result{Roots: slices.Clone(roots), Added: added, Candidates: candidates, Unreadable: s.unreadable, DockerData: docker.Data(), StaleAfter: settings.StaleAfter}
}

func (s *scanner) placeCaches(ecosystems []domain.Ecosystem, home string, given []domain.Root) []domain.Root {
	if home == "" {
		return nil
	}
	var added []domain.Root
	for _, e := range ecosystems {
		for _, r := range e.Rules {
			w, ok := r.Where.(domain.InSharedCache)
			if !ok {
				continue
			}
			full, err := s.insideHome(home, cacheLocation(w))
			if err != nil {
				if !errors.Is(err, errNotAdded) && !s.ignored(full) {
					s.unread(full, err)
				}
				continue
			}
			got := placed{recogniser{ecosystem: e.Name, rule: r}}
			s.caches[full] = got
			space, err := s.files.Space(full)
			if err != nil {
				if !s.ignored(full) {
					s.unread(full, err)
				}
				continue
			}
			s.cacheID[space.File] = got
			if spelling := rootSpelling(s.files, given, space.File); spelling != "" {
				s.caches[spelling] = got
				continue
			}
			added = append(added, domain.Root(full))
			given = append(given, domain.Root(full))
		}
	}
	return added
}

func rootSpelling(files Files, given []domain.Root, id FileID) string {
	for _, root := range given {
		space, err := files.Space(string(root))
		if err != nil || space.File != id {
			continue
		}
		return string(root)
	}
	return ""
}

var errNotAdded = errors.New("not added")

func (s *scanner) insideHome(home, location string) (string, error) {
	if !cleanRelative(location) || s.ignored(home) {
		return "", errNotAdded
	}
	// A home that is itself a Link is followed once. Listing it fails, and
	// that failure is not a directory the Scan could not read. A Link in
	// an ancestor is already followed by the files adapter.
	home, err := s.followHome(home)
	if err != nil {
		return home, err
	}
	cur := home
	for _, part := range strings.Split(location, "/") {
		entries, err := s.files.List(cur)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || s.ignored(cur) {
				return "", errNotAdded
			}
			return cur, err
		}
		var next Entry
		var found bool
		for _, e := range entries {
			if sameName(e.Name, part) {
				next, found = e, true
				break
			}
		}
		if !found || next.Is != Dir || s.ignored(next.Path) || !safety.Beneath(home, next.Path, false) || next.Path == home {
			return "", errNotAdded
		}
		cur = next.Path
	}
	return cur, nil
}

// followHome is path when it can be listed, or the directory a Link at
// path leads to. A missing path is errNotAdded. Any other failure is
// returned so the caller can report it.
func (s *scanner) followHome(path string) (string, error) {
	if _, err := s.files.List(path); err == nil {
		return path, nil
	} else if errors.Is(err, fs.ErrNotExist) || s.ignored(path) {
		return "", errNotAdded
	} else if links, ok := s.files.(eachLink); ok {
		if full, ferr := links.EachLink(path); ferr == nil {
			if _, lerr := s.files.List(full); lerr == nil {
				return full, nil
			}
		}
		return path, err
	} else {
		return path, err
	}
}

func (s *scanner) shared(root, path string, c placed) {
	space, err := s.files.Space(path)
	if err != nil {
		s.unread(path, err)
		return
	}
	if s.walked[space.File] {
		return
	}
	s.walked[space.File] = true
	w := c.recogniser.rule.Where.(domain.InSharedCache)
	if !w.Split {
		s.candidate(root, Entry{Path: path, Is: Dir}, c.recogniser, "")
		return
	}
	entries, err := s.files.List(path)
	if err != nil {
		s.unread(path, err)
		return
	}
	for _, e := range entries {
		if s.ignored(e.Path) || e.Is == Link || (e.Is != Dir && e.Is != File) || !safety.Beneath(root, e.Path, false) {
			continue
		}
		s.candidate(root, e, c.recogniser, "")
	}
}

func cacheLocation(w domain.InSharedCache) string {
	switch runtime.GOOS {
	case "windows":
		return w.Windows
	case "darwin", "ios":
		return w.MacOS
	default:
		return w.Linux
	}
}

func cleanRelative(location string) bool {
	return location != "" &&
		!path.IsAbs(location) &&
		path.Clean(location) == location &&
		location != "." && location != ".." && !strings.HasPrefix(location, "../") &&
		!strings.HasPrefix(location, "~") &&
		!strings.ContainsAny(location, `\:$%`)
}

func sameName(name, part string) bool {
	if name == part {
		return true
	}
	switch runtime.GOOS {
	case "windows", "darwin", "ios":
		return strings.EqualFold(name, part)
	default:
		return false
	}
}

type scanner struct {
	files         Files
	git           safety.Git
	rules         rules
	caches        map[string]placed
	cacheID       map[FileID]placed
	fileCandidate map[string]bool
	walked        map[FileID]bool
	listed        map[FileID]bool
	inside        map[FileID]bool
	ignore        []string
	formOf        map[string]string

	found      []found
	unreadable []Unreadable
}

type placed struct {
	recogniser recogniser
}

type found struct {
	id        FileID
	candidate domain.Candidate
}

func (s *scanner) enter(root, path string) {
	if s.ignored(path) || !safety.Beneath(root, path, false) {
		return
	}
	if c, ok := s.caches[path]; ok {
		s.shared(root, path, c)
		return
	}
	space, err := s.files.Space(path)
	if err != nil {
		s.unread(path, err)
		return
	}
	if c, ok := s.cacheID[space.File]; ok {
		s.shared(root, path, c)
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
	measured := domain.Size(space.Bytes)
	if dir.Is == File {
		s.fileCandidate[dir.Path] = true
	} else {
		measured = s.size(root, dir.Path, space)
	}
	s.found = append(s.found, found{space.File, domain.Candidate{
		Path:      dir.Path,
		Size:      measured,
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
		if s.fileCandidate[c.Path] {
			mod, err := s.files.Modified(c.Path)
			if err != nil {
				s.missed(c.Path, err)
				blocked = true
			} else {
				used = mod
			}
		} else if c.Project == "" {
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

// eachLink follows every Link in a path. The files adapter has it. A fake
// does not, and then an Ignore path is compared as written.
type eachLink interface {
	EachLink(string) (string, error)
}

func (s *scanner) ignored(path string) bool {
	if len(s.ignore) == 0 {
		return false
	}
	for _, p := range s.ignore {
		if safety.Beneath(p, path, false) {
			return true
		}
	}
	walked := s.form(path)
	for _, p := range s.ignore {
		if safety.Beneath(s.form(p), walked, false) {
			return true
		}
	}
	return false
}

// form is path with every Link followed, when the files port can follow
// them. ignored calls it only after the spellings do not match, and only
// when the Ignore list is not empty. An Ignore path and a walked path are
// then compared in that same form, because a Root keeps an earlier Link in
// the caller's spelling.
func (s *scanner) form(path string) string {
	if got, ok := s.formOf[path]; ok {
		return got
	}
	out := path
	if links, ok := s.files.(eachLink); ok {
		if full, err := links.EachLink(path); err == nil {
			out = full
		}
	}
	s.formOf[path] = out
	return out
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
