// Package files reads the real file tree for a Scan. It only reads.
package files

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// Disk is the scan.Files of the machine sweep runs on.
type Disk struct{}

var _ scan.Files = Disk{}

// List reads one directory. os.ReadDir gives the names in order, and each
// one's type from the directory read itself, without following it.
func (Disk) List(dir string) ([]scan.Entry, error) {
	inside, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]scan.Entry, len(inside))
	for i, e := range inside {
		entries[i] = scan.Entry{Name: e.Name(), Path: filepath.Join(dir, e.Name()), Is: is(e.Type())}
	}
	return entries, nil
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
