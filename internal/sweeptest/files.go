package sweeptest

import (
	"errors"
	"io/fs"
	"path"
	"slices"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// Top is where the fake holds its Tree.
const Top = "/top"

// DirSpace is the Space of every directory in the fake.
const DirSpace = 4096

// Files is the fake of scan.Files: a Tree held in memory. A File's Space is
// its Bytes rounded up to 512, a directory's is DirSpace and a Link's is
// nothing. Its paths are slash-separated on every operating system. It never
// follows a Link, so listing one fails.
type Files struct {
	nodes      map[string]*fakeNode
	unreadable map[string]error
}

type fakeNode struct {
	is       scan.Is
	space    scan.Space
	modified time.Time
	inside   []string
}

var _ scan.Files = (*Files)(nil)

var errNotADirectory = errors.New("not a directory")

// NewFiles holds the Tree beneath Top.
func NewFiles(tree Tree) *Files {
	f := &Files{nodes: map[string]*fakeNode{}, unreadable: map[string]error{}}
	var files uint64
	node := func(is scan.Is, bytes int64) *fakeNode {
		files++
		return &fakeNode{is: is, space: scan.Space{Bytes: (bytes + 511) / 512 * 512, File: scan.FileID{Volume: 1, File: files}}}
	}
	var place func(p string, n *fakeNode)
	place = func(p string, n *fakeNode) {
		if _, ok := f.nodes[p]; ok {
			return
		}
		f.nodes[p] = n
		if p == Top {
			return
		}
		place(path.Dir(p), node(scan.Dir, DirSpace))
		parent := f.nodes[path.Dir(p)]
		parent.inside = append(parent.inside, path.Base(p))
		slices.Sort(parent.inside)
	}
	place(Top, node(scan.Dir, DirSpace))
	for _, p := range tree.inOrder() {
		at := path.Join(Top, p)
		switch n := tree[p].(type) {
		case File:
			made := node(scan.File, int64(n.Bytes))
			made.modified = n.Modified
			place(at, made)
		case HardLink:
			place(at, f.nodes[path.Join(Top, n.To)])
		case Link:
			place(at, node(scan.Link, 0))
		case Dir:
			place(at, node(scan.Dir, DirSpace))
		}
	}
	return f
}

// Unreadable makes List and Space fail with err at the path p beneath Top.
func (f *Files) Unreadable(p string, err error) { f.unreadable[path.Join(Top, p)] = err }

func (f *Files) List(dir string) ([]scan.Entry, error) {
	n, err := f.at("list", dir)
	if err != nil {
		return nil, err
	}
	if n.is != scan.Dir {
		return nil, &fs.PathError{Op: "list", Path: dir, Err: errNotADirectory}
	}
	entries := make([]scan.Entry, len(n.inside))
	for i, name := range n.inside {
		p := dir + "/" + name
		entries[i] = scan.Entry{Name: name, Path: p, Is: f.nodes[p].is}
	}
	return entries, nil
}

func (f *Files) Space(p string) (scan.Space, error) {
	n, err := f.at("space", p)
	if err != nil {
		return scan.Space{}, err
	}
	return n.space, nil
}

func (f *Files) Modified(p string) (time.Time, error) {
	n, err := f.at("modified", p)
	if err != nil {
		return time.Time{}, err
	}
	return n.modified, nil
}

func (f *Files) at(op, p string) (*fakeNode, error) {
	if err, ok := f.unreadable[p]; ok {
		return nil, &fs.PathError{Op: op, Path: p, Err: err}
	}
	n, ok := f.nodes[p]
	if !ok {
		return nil, &fs.PathError{Op: op, Path: p, Err: fs.ErrNotExist}
	}
	return n, nil
}
