package domain

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// Validate reports every way the Ecosystems break the model, nil if none.
// Some breaks show only across Ecosystems that are each right on their own,
// such as two Rules that would list the same Candidate twice, so it takes
// them all at once.
func Validate(ecosystems []Ecosystem) error {
	var errs []error
	names := map[string]bool{}
	projects := map[InProject]string{}
	var caches []sharedCache
	for _, e := range ecosystems {
		if strings.TrimSpace(e.Name) == "" {
			errs = append(errs, errors.New("an Ecosystem has no Name"))
		}
		if names[strings.ToLower(e.Name)] {
			errs = append(errs, fmt.Errorf("two Ecosystems are named %q", e.Name))
		}
		names[strings.ToLower(e.Name)] = true
		if len(e.Rules) == 0 {
			errs = append(errs, fmt.Errorf("Ecosystem %q has no Rules", e.Name))
		}
		for i, r := range e.Rules {
			at := fmt.Sprintf("Ecosystem %q Rule %d", e.Name, i+1)
			fail := func(why string) { errs = append(errs, errors.New(at+" "+why)) }
			if !known(r.Kind) {
				fail(fmt.Sprintf("names Kind %q, which is not one of %v", r.Kind, kinds))
			}
			if strings.TrimSpace(string(r.Reason)) == "" {
				fail("has no Reason")
			}
			switch w := r.Where.(type) {
			case InProject:
				if !oneName(w.Manifest) || !oneName(w.Candidate) {
					fail("needs a Manifest and a Candidate that are each one file name")
				}
				if other, ok := projects[w]; ok {
					fail("finds the same Candidate as " + other)
				}
				projects[w] = at
			case InSharedCache:
				if w.Linux == "" && w.MacOS == "" && w.Windows == "" {
					fail("has no Shared cache location on any operating system")
				}
				for _, c := range []sharedCache{{"Linux", w.Linux, at}, {"MacOS", w.MacOS, at}, {"Windows", w.Windows, at}} {
					if c.location == "" {
						continue
					}
					if !insideHome(c.location) {
						fail(fmt.Sprintf("has %s location %q, which is not a clean slash-separated path inside the home directory", c.system, c.location))
					}
					caches = append(caches, c)
				}
			default:
				fail("has no Where")
			}
		}
	}
	for i, a := range caches {
		for _, b := range caches[i+1:] {
			if overlap(a, b) {
				errs = append(errs, fmt.Errorf("%s and %s have Shared caches at %q and %q on %s, and a Candidate never holds another", a.owner, b.owner, a.location, b.location, a.system))
			}
		}
	}
	return errors.Join(errs...)
}

type sharedCache struct{ system, location, owner string }

// overlap folds case on macOS and Windows, whose file systems ignore it by
// default, so two spellings of one place are one place.
func overlap(a, b sharedCache) bool {
	if a.system != b.system {
		return false
	}
	p, q := a.location, b.location
	if a.system != "Linux" {
		p, q = strings.ToLower(p), strings.ToLower(q)
	}
	return beneath(p, q) || beneath(q, p)
}

func known(k Kind) bool {
	for _, each := range kinds {
		if k == each {
			return true
		}
	}
	return false
}

func oneName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

func insideHome(location string) bool {
	return !path.IsAbs(location) &&
		path.Clean(location) == location &&
		location != "." && location != ".." && !strings.HasPrefix(location, "../") &&
		!strings.HasPrefix(location, "~") &&
		!strings.ContainsAny(location, `\:$%`)
}

func beneath(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+"/")
}
