// Package sweeptest holds the fakes of the domain's ports and the contract
// suites that every adapter and its fake must pass.
package sweeptest

import (
	"cmp"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// Tree is a small file tree written once, as data, and built twice: in
// memory by NewFiles and on disk by OnDisk.
//
// A key is a slash-separated path beneath the top of the tree. Parent
// directories are made for it.
type Tree map[string]Node

// Node is a File, a HardLink, a Link or a Dir, and nothing else compiles.
type Node interface{ node() }

// File is a regular file that holds this many bytes.
type File struct{ Bytes int }

// HardLink is another name for the File at the path To.
type HardLink struct{ To string }

// Link leads to the path To in the Tree, which need not be there.
type Link struct{ To string }

// Dir is a directory with nothing in it.
type Dir struct{}

func (File) node()     {}
func (HardLink) node() {}
func (Link) node()     {}
func (Dir) node()      {}

func (tree Tree) inOrder() []string {
	rank := func(p string) int {
		switch tree[p].(type) {
		case HardLink:
			return 1
		case Link:
			return 2
		}
		return 0
	}
	paths := make([]string, 0, len(tree))
	for p := range tree {
		paths = append(paths, p)
	}
	slices.SortFunc(paths, func(a, b string) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(a, b))
	})
	return paths
}

// OnDisk builds the Tree in a new temporary directory and returns that
// directory.
//
// A Link is a symlink. Windows lets an account make one only with Developer
// Mode on or as an administrator, so there a test is skipped when the
// account may not, and fails instead when CI is set, where it may.
func OnDisk(t *testing.T, tree Tree) string {
	t.Helper()
	top := t.TempDir()
	at := func(p string) string { return filepath.Join(top, filepath.FromSlash(p)) }
	for _, p := range tree.inOrder() {
		if err := os.MkdirAll(filepath.Dir(at(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		var err error
		switch n := tree[p].(type) {
		case File:
			err = write(at(p), n.Bytes)
		case HardLink:
			err = os.Link(at(n.To), at(p))
		case Link:
			err = os.Symlink(at(n.To), at(p))
			if err != nil && runtime.GOOS == "windows" && os.Getenv("CI") == "" {
				t.Skipf("this account may not make a symlink: %v", err)
			}
		case Dir:
			err = os.Mkdir(at(p), 0o755)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return top
}

// write fills a file with bytes no file system can compress or store
// sparsely, the same on every run, and syncs it, so its Space is already
// what the file system will keep and is never less than its length.
func write(path string, n int) error {
	data := make([]byte, n)
	_, _ = rand.NewChaCha8([32]byte{}).Read(data)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
