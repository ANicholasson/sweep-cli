// Package clean carries out a Plan. It reads the file tree only through
// scan.Files, asks the Git rule only through safety.Git, and moves a
// Candidate only through Trash.
package clean

import (
	"errors"
	"io/fs"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/plan"
	"github.com/ANicholasson/sweep-cli/internal/domain/safety"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// Trash moves a path into the operating system's trash, where it can be
// restored. Move never deletes. When Move returns an error, the path is
// still where it was.
type Trash interface {
	Move(path string) error
	Restore(path string) error
}

// Rule is the Safety rule that made a Skip.
type Rule uint8

const (
	// Git is the Git rule.
	Git Rule = iota + 1
	// Root is the Root rule.
	Root
	// Fingerprint is the Fingerprint rule.
	Fingerprint
)

// String is the rule's name as the glossary writes it.
func (r Rule) String() string {
	switch r {
	case Git:
		return "Git rule"
	case Root:
		return "Root rule"
	case Fingerprint:
		return "Fingerprint rule"
	default:
		return ""
	}
}

// Item is one Candidate a Clean acted on, with the Size the Plan recorded.
type Item struct {
	Path string
	Size domain.Size
}

// Skip is a Candidate a Safety rule kept. Nothing inside it was touched.
type Skip struct {
	Item
	Rule Rule
}

// Failure is a Candidate that could not be moved. It is still in place.
type Failure struct {
	Item
	Err error
}

// Result is what a Clean did. Trashed space was moved to the trash. It is
// not reclaimed.
type Result struct {
	Trashed []Item
	Skipped []Skip
	Failed  []Failure
}

type reached int

const (
	reachOK reached = iota
	reachOutside
	reachLink
	reachGone
	reachBlocked
)

// Run carries out p. Each Candidate is judged on its own, immediately
// before it would be moved, and one Candidate does not stop the rest.
//
// The Root rule comes first. The Candidate is reached from a Root one
// component at a time, and a Link on that walk is a Skip. Nothing under
// the Link is listed, so a directory above the Candidate that was swapped
// for a Link is not followed.
//
// The Git rule is next. A path the Git rule keeps is a Skip, and so is a
// Candidate that holds a directory that cannot be read, because a
// repository with tracked files could be inside it.
//
// The Fingerprint rule is last. A Candidate whose Fingerprint no longer
// matches is a Skip.
//
// Only then does Move run. Move never deletes. When it fails, the
// Candidate stays and the failure is reported.
func Run(files scan.Files, git safety.Git, bin Trash, p plan.Plan) Result {
	var out Result
	for _, c := range p.Picked {
		item := Item{Path: c.Path, Size: c.Fingerprint.Size}
		switch reach(files, p.Roots, c.Path) {
		case reachOutside, reachLink, reachBlocked:
			out.Skipped = append(out.Skipped, Skip{Item: item, Rule: Root})
			continue
		case reachGone:
			out.Skipped = append(out.Skipped, Skip{Item: item, Rule: Fingerprint})
			continue
		}
		if safety.Keeps(git, c.Path) || !readable(files, c.Path) {
			out.Skipped = append(out.Skipped, Skip{Item: item, Rule: Git})
			continue
		}
		same, err := plan.Match(files, c)
		if err != nil || !same {
			out.Skipped = append(out.Skipped, Skip{Item: item, Rule: Fingerprint})
			continue
		}
		if err := bin.Move(c.Path); err != nil {
			out.Failed = append(out.Failed, Failure{Item: item, Err: err})
			continue
		}
		out.Trashed = append(out.Trashed, item)
	}
	return out
}

// reach walks from the shallowest Root that contains candidate, one
// component at a time. It lists only a directory it has already seen is
// not a Link, and it takes the next path from that listing.
func reach(files scan.Files, roots []domain.Root, candidate string) reached {
	root, ok := shallowest(roots, candidate)
	if !ok {
		return reachOutside
	}
	parts, ok := steps(string(root), candidate)
	if !ok {
		return reachOutside
	}
	current := string(root)
	if len(parts) == 0 {
		return listed(files, current)
	}
	for i, name := range parts {
		entries, err := files.List(current)
		if err != nil {
			return listedErr(err)
		}
		var found scan.Entry
		seen := false
		for _, e := range entries {
			if e.Name == name {
				found = e
				seen = true
				break
			}
		}
		if !seen {
			return reachGone
		}
		if found.Is == scan.Link {
			return reachLink
		}
		if found.Is != scan.Dir && found.Is != scan.File {
			return reachBlocked
		}
		if i < len(parts)-1 {
			if found.Is != scan.Dir {
				return reachGone
			}
			current = found.Path
		}
	}
	return reachOK
}

func listed(files scan.Files, dir string) reached {
	if _, err := files.List(dir); err != nil {
		return listedErr(err)
	}
	return reachOK
}

func listedErr(err error) reached {
	if errors.Is(err, fs.ErrNotExist) {
		return reachGone
	}
	return reachBlocked
}

func shallowest(roots []domain.Root, candidate string) (domain.Root, bool) {
	var best domain.Root
	bestN := -1
	for _, root := range roots {
		if !safety.Beneath(string(root), candidate, false) {
			continue
		}
		_, parts, ok := safety.Split(string(root))
		if !ok {
			continue
		}
		if bestN < 0 || len(parts) < bestN {
			best = root
			bestN = len(parts)
		}
	}
	return best, bestN >= 0
}

func steps(root, candidate string) ([]string, bool) {
	_, rp, rok := safety.Split(root)
	_, cp, cok := safety.Split(candidate)
	if !rok || !cok || len(cp) < len(rp) {
		return nil, false
	}
	for i := range rp {
		if rp[i] != cp[i] {
			return nil, false
		}
	}
	return cp[len(rp):], true
}

// readable reports whether every directory inside path can be listed.
// A Link is not entered. A directory that cannot be listed hides whatever
// is inside it, so the caller keeps the Candidate.
func readable(files scan.Files, path string) bool {
	seen := map[scan.FileID]bool{}
	var walk func(string) bool
	walk = func(dir string) bool {
		entries, err := files.List(dir)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.Is != scan.Dir || !safety.Beneath(path, e.Path, false) {
				continue
			}
			space, err := files.Space(e.Path)
			if err != nil {
				return false
			}
			if seen[space.File] {
				continue
			}
			seen[space.File] = true
			if !walk(e.Path) {
				return false
			}
		}
		return true
	}
	return walk(path)
}
