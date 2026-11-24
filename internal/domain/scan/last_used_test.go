package scan

import (
	"io/fs"
	"testing"
	"time"
)

// A Candidate with no Project takes the newest modification inside it.
// The built binary cannot show one. Every Candidate a Rule finds sits
// beside a manifest, so it has a Project, and a Scan does not yet find
// Shared caches. This drives the walk Scan uses when Project is empty.
func TestACandidateWithNoProjectTakesTheNewestModificationInsideIt(t *testing.T) {
	older := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(2 * time.Hour)
	later := newer.Add(time.Hour)
	const (
		entry     = "/cache/entry"
		oldFile   = entry + "/old.bin"
		newFile   = entry + "/deep/new.bin"
		link      = entry + "/jump"
		elsewhere = "/elsewhere/later.bin"
	)
	files := memFiles{
		dirs: map[string][]Entry{
			entry: {
				{Name: "deep", Path: entry + "/deep", Is: Dir},
				{Name: "jump", Path: link, Is: Link},
				{Name: "old.bin", Path: oldFile, Is: File},
			},
			entry + "/deep": {{Name: "new.bin", Path: newFile, Is: File}},
			"/elsewhere":    {{Name: "later.bin", Path: elsewhere, Is: File}},
		},
		mod: map[string]time.Time{oldFile: older, newFile: newer, elsewhere: later},
	}
	s := scanner{files: files}

	got, blocked := s.newest(entry, nil)

	if blocked || !got.Equal(newer) {
		t.Errorf("Last used = %s, want %s, the newest file inside the Candidate", got, newer)
	}
	if len(s.unreadable) != 0 {
		t.Errorf("Unreadable = %v, want nothing", s.unreadable)
	}
}

// memFiles is the list and the modification time one walk needs. The shared
// fake imports this package, so this test cannot use it.
type memFiles struct {
	dirs map[string][]Entry
	mod  map[string]time.Time
}

func (m memFiles) List(dir string) ([]Entry, error) {
	entries, ok := m.dirs[dir]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return entries, nil
}

func (m memFiles) Space(string) (Space, error) { return Space{}, nil }

func (m memFiles) Modified(path string) (time.Time, error) {
	when, ok := m.mod[path]
	if !ok {
		return time.Time{}, fs.ErrNotExist
	}
	return when, nil
}
