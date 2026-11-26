// Package plan chooses the Candidates one Clean will act on and records the
// Fingerprint of each. It reads the file tree only through scan.Files, and
// it has no method that writes.
package plan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/safety"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// UnknownPath is a path named for the Plan that is not a Candidate.
type UnknownPath struct{ Path string }

func (e UnknownPath) Error() string { return e.Path + " is not a Candidate" }

// UnknownEcosystem is an Ecosystem named for a Pick that has no Shared cache
// in the Scan.
type UnknownEcosystem struct{ Name string }

func (e UnknownEcosystem) Error() string { return e.Name + " has no Shared cache" }

// Fingerprint is what a Plan records about one Candidate so a later Clean
// can see that the Candidate changed. ID is the file identity of the
// Candidate path itself. Entries counts every directory entry inside it,
// and a Link counts as one entry and is not followed. Size counts each
// hard-linked file once, the same way a Scan measures a Size, and does not
// follow a Link. Newest is
// the newest modification of the Candidate and of every path inside it. A
// Link's own time counts, and what the Link leads to does not.
type Fingerprint struct {
	ID      scan.FileID
	Entries int64
	Size    domain.Size
	Newest  time.Time
}

// Picked is one Candidate the Plan will act on.
type Picked struct {
	Path        string
	Fingerprint Fingerprint
}

// Plan is the exact list of Candidates one Clean will act on, and the Roots
// it was made from.
type Plan struct {
	Roots  []domain.Root
	Picked []Picked
}

type Choice struct {
	Candidate domain.Candidate
	Picked    bool
	Reason    string
}

// Pick is the choice one dry run asked for. Named are paths from --pick.
// Kinds and Ecosystems narrow the choice; an empty list does not. AnyAge
// chooses every Candidate those lists allow. StaleNamed means this Scan's
// threshold was given as --stale-days, and the reason uses Result.StaleAfter.
// Caches are Ecosystems named with --pick-caches. A Shared cache of one of
// them is picked. An Ecosystem in Caches with no Shared cache in the Scan
// is an UnknownEcosystem. None of the narrowing choices picks a Shared cache.
type Pick struct {
	Named      []string
	Kinds      []domain.Kind
	Ecosystems []string
	Caches     []string
	AnyAge     bool
	StaleNamed bool
}

// Build chooses the Candidates one Plan will act on. A path in pick.Named
// is matched by file identity. A path that is not a Candidate is an
// UnknownPath. A Shared cache is chosen when it is named, or when its
// Ecosystem is in pick.Caches. One that is left out is "choose it yourself".
func Build(files scan.Files, result scan.Result, pick Pick) (Plan, []Choice, error) {
	type namedFile struct {
		path string
		id   scan.FileID
	}
	var wanted []namedFile
	namedID := map[scan.FileID]bool{}
	for _, p := range pick.Named {
		space, err := files.Space(p)
		if err != nil {
			return Plan{}, nil, UnknownPath{Path: p}
		}
		wanted = append(wanted, namedFile{p, space.File})
		namedID[space.File] = true
	}

	seen := map[scan.FileID]bool{}
	seenEco := map[string]bool{}
	var choices []Choice
	var picked []Picked
	for _, c := range result.Candidates {
		space, err := files.Space(c.Path)
		if err != nil {
			return Plan{}, nil, fmt.Errorf("%s: %w", c.Path, err)
		}
		chosen := namedID[space.File]
		if chosen {
			seen[space.File] = true
		}
		if c.Rule.SharedCache() {
			seenEco[strings.ToLower(c.Ecosystem)] = true
		}
		take, reason := decide(c, chosen, pick, result.StaleAfter)
		choices = append(choices, Choice{Candidate: c, Picked: take, Reason: reason})
		if !take {
			continue
		}
		measured, err := FingerprintOf(files, c.Path)
		if err != nil {
			return Plan{}, nil, fmt.Errorf("%s: %w", c.Path, err)
		}
		picked = append(picked, Picked{Path: c.Path, Fingerprint: measured})
	}
	for _, w := range wanted {
		if !seen[w.id] {
			return Plan{}, nil, UnknownPath{Path: w.path}
		}
	}
	for _, name := range pick.Caches {
		if !seenEco[strings.ToLower(name)] {
			return Plan{}, nil, UnknownEcosystem{Name: name}
		}
	}
	return Plan{Roots: append([]domain.Root(nil), result.Roots...), Picked: picked}, choices, nil
}

func decide(c domain.Candidate, named bool, pick Pick, staleAfter time.Duration) (bool, string) {
	if named {
		return true, "named with --pick"
	}
	if c.Rule.SharedCache() {
		if ecosystemNamed(pick.Caches, c.Ecosystem) {
			return true, "named with --pick-caches"
		}
		return false, "choose it yourself"
	}
	if len(pick.Kinds) > 0 && !slices.Contains(pick.Kinds, c.Rule.Kind) {
		return false, "not named with --kind"
	}
	if len(pick.Ecosystems) > 0 && !slices.Contains(pick.Ecosystems, c.Ecosystem) {
		return false, "not named with --ecosystem"
	}
	if pick.AnyAge {
		return true, "--all"
	}
	if pick.StaleNamed {
		days := int(staleAfter / (24 * time.Hour))
		if c.Stale {
			return true, fmt.Sprintf("older than --stale-days %d", days)
		}
		return false, fmt.Sprintf("newer than --stale-days %d", days)
	}
	if c.Stale {
		return true, "Stale"
	}
	return false, "used recently"
}

func ecosystemNamed(names []string, ecosystem string) bool {
	for _, name := range names {
		if strings.EqualFold(name, ecosystem) {
			return true
		}
	}
	return false
}

// FingerprintOf measures path the way a Plan records it. Size counts each
// hard-linked file once and does not follow a Link. A Link counts as one
// entry.
func FingerprintOf(files scan.Files, path string) (Fingerprint, error) {
	space, err := files.Space(path)
	if err != nil {
		return Fingerprint{}, err
	}
	newest, err := files.Modified(path)
	if err != nil {
		return Fingerprint{}, err
	}
	entered := map[scan.FileID]bool{space.File: true}
	counted := map[scan.FileID]bool{space.File: true}
	size := space.Bytes
	var entries int64
	var walk func(string) error
	walk = func(dir string) error {
		listed, err := files.List(dir)
		if err != nil {
			return err
		}
		for _, e := range listed {
			if !safety.Beneath(path, e.Path, false) {
				continue
			}
			entries++
			mod, err := files.Modified(e.Path)
			if err != nil {
				return err
			}
			if mod.After(newest) {
				newest = mod
			}
			id, err := files.Space(e.Path)
			if err != nil {
				return err
			}
			if !counted[id.File] {
				counted[id.File] = true
				size += id.Bytes
			}
			if e.Is != scan.Dir || entered[id.File] {
				continue
			}
			entered[id.File] = true
			if err := walk(e.Path); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(path); err != nil {
		return Fingerprint{}, err
	}
	return Fingerprint{ID: space.File, Entries: entries, Size: domain.Size(size), Newest: newest}, nil
}

// Match reports whether path still has the Fingerprint a Plan recorded.
// An error means the Fingerprint could not be read, so it does not match.
func Match(files scan.Files, picked Picked) (bool, error) {
	got, err := FingerprintOf(files, picked.Path)
	if err != nil {
		return false, err
	}
	want := picked.Fingerprint
	return got.ID == want.ID && got.Entries == want.Entries && got.Size == want.Size && got.Newest.Equal(want.Newest), nil
}

// Every list is [] when it is empty, never null.
type planV1 struct {
	Version    int        `json:"version"`
	Roots      []string   `json:"roots"`
	Candidates []pickedV1 `json:"candidates"`
}

type pickedV1 struct {
	Path        string        `json:"path"`
	Fingerprint fingerprintV1 `json:"fingerprint"`
}

type fingerprintV1 struct {
	Identity identityV1 `json:"identity"`
	Entries  int64      `json:"entries"`
	Size     int64      `json:"size"`
	Newest   time.Time  `json:"newest"`
}

type identityV1 struct {
	Volume uint64 `json:"volume"`
	File   uint64 `json:"file"`
}

// Is reports whether data is a version 1 Plan and nothing else. One JSON
// value must end the file.
func Is(data []byte) bool {
	raw, err := decodePlan(data)
	return err == nil && raw.Version == 1
}

// decodePlan reads one JSON value and requires the file to end there.
// Decoder.More only looks for another element inside the value just read,
// so a Plan followed by "}garbage" would pass that check.
func decodePlan(data []byte) (planV1, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var raw planV1
	if err := dec.Decode(&raw); err != nil {
		return planV1{}, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return planV1{}, errors.New("the Plan is malformed")
	}
	return raw, nil
}

// Marshal writes a Plan as version 1 JSON. The bytes end with a newline.
func Marshal(p Plan) ([]byte, error) {
	out := planV1{Version: 1, Roots: []string{}, Candidates: []pickedV1{}}
	for _, root := range p.Roots {
		out.Roots = append(out.Roots, string(root))
	}
	for _, c := range p.Picked {
		out.Candidates = append(out.Candidates, pickedV1{
			Path: c.Path,
			Fingerprint: fingerprintV1{
				Identity: identityV1{Volume: c.Fingerprint.ID.Volume, File: c.Fingerprint.ID.File},
				Entries:  c.Fingerprint.Entries,
				Size:     int64(c.Fingerprint.Size),
				Newest:   c.Fingerprint.Newest.UTC(),
			},
		})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unmarshal reads a version 1 Plan. A malformed Plan, a Plan with a field
// this version does not have, or a Plan of another version is refused.
func Unmarshal(data []byte) (Plan, error) {
	raw, err := decodePlan(data)
	if err != nil {
		return Plan{}, errors.New("the Plan is malformed")
	}
	if raw.Version != 1 {
		return Plan{}, fmt.Errorf("the Plan has version %d, and sweep reads version 1", raw.Version)
	}
	out := Plan{Roots: []domain.Root{}, Picked: []Picked{}}
	for _, root := range raw.Roots {
		if err := plainPath(root); err != nil {
			return Plan{}, err
		}
		out.Roots = append(out.Roots, domain.Root(root))
	}
	for _, c := range raw.Candidates {
		if err := plainPath(c.Path); err != nil {
			return Plan{}, err
		}
		out.Picked = append(out.Picked, Picked{
			Path: c.Path,
			Fingerprint: Fingerprint{
				ID:      scan.FileID{Volume: c.Fingerprint.Identity.Volume, File: c.Fingerprint.Identity.File},
				Entries: c.Fingerprint.Entries,
				Size:    domain.Size(c.Fingerprint.Size),
				Newest:  c.Fingerprint.Newest,
			},
		})
	}
	return out, nil
}

// plainPath accepts only an absolute path that filepath.Clean would leave
// unchanged. A ".." is cleaned in the text of the path before a move, and
// followed by the kernel when a rule is re-checked, so the two can name
// different directories when a Link is on the way. Such a path is refused.
func plainPath(p string) error {
	if p == "" {
		return errors.New("the Plan is malformed")
	}
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return fmt.Errorf("the Plan path %s is not a clean absolute path", p)
	}
	return nil
}
