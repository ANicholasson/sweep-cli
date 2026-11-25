// Package files reads the real file tree for a Scan. It only reads.
package files

import (
	"cmp"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// Disk is the scan.Files of the machine sweep runs on.
type Disk struct{}

var _ scan.Files = Disk{}

// List reads one directory without following a Link at the end of dir, so a
// directory swapped for a Link while a Scan runs is not followed. Names come
// back in order, and each one's type comes from the directory read itself.
func (Disk) List(dir string) ([]scan.Entry, error) {
	f, err := openDir(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	inside, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(inside, func(a, b fs.DirEntry) int { return cmp.Compare(a.Name(), b.Name()) })
	entries := make([]scan.Entry, len(inside))
	for i, e := range inside {
		entries[i] = scan.Entry{Name: e.Name(), Path: filepath.Join(dir, e.Name()), Is: is(e.Type())}
	}
	return entries, nil
}

// Modified is lstat's modification time. The access time is never read.
func (Disk) Modified(path string) (time.Time, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// is reads a Link from the two bits the os package sets for one. A symlink
// has ModeSymlink on every operating system. On Windows a junction has
// ModeIrregular and no ModeDir, and so does every reparse point the os
// package does not know, such as a cloud placeholder. sweep cannot tell
// where those lead, so they are Links too, and a Scan stays out of them.
func is(mode fs.FileMode) scan.Is {
	switch {
	case mode&(fs.ModeSymlink|fs.ModeIrregular) != 0:
		return scan.Link
	case mode.IsDir():
		return scan.Dir
	case mode.IsRegular():
		return scan.File
	default:
		return scan.Other
	}
}
