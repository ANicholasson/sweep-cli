package sweeptest

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/ANicholasson/sweep-cli/internal/domain/clean"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// Trash is the fake of clean.Trash. It moves paths inside Files and can
// put them back as the same files. Fail, when set for a path, makes Move
// return that error and leave the path in place. Before runs at the start
// of Move, which is after a Clean has re-checked the path.
type Trash struct {
	Files  *Files
	Fail   map[string]error
	Before func(path string)
	Moved  []string
	held   map[string][]held
}

type held struct {
	name   string
	parent string
	nodes  map[string]*fakeNode
}

var _ clean.Trash = (*Trash)(nil)

// NewTrash moves paths in files.
func NewTrash(files *Files) *Trash {
	return &Trash{Files: files, held: map[string][]held{}}
}

// Move takes path out of Files. A path in Fail is left in place.
func (t *Trash) Move(p string) error {
	if t.Before != nil {
		t.Before(p)
	}
	if err := t.Fail[p]; err != nil {
		return err
	}
	parent := path.Dir(p)
	name := path.Base(p)
	pn := t.Files.nodes[parent]
	if pn == nil || t.Files.nodes[p] == nil || slices.Index(pn.inside, name) < 0 {
		return &fs.PathError{Op: "move", Path: p, Err: fs.ErrNotExist}
	}
	saved := map[string]*fakeNode{}
	prefix := p + "/"
	for q, n := range t.Files.nodes {
		if q == p || strings.HasPrefix(q, prefix) {
			saved[q] = n
		}
	}
	for q := range saved {
		delete(t.Files.nodes, q)
	}
	i := slices.Index(pn.inside, name)
	pn.inside = slices.Delete(pn.inside, i, i+1)
	t.held[p] = append(t.held[p], held{name: name, parent: parent, nodes: saved})
	t.Moved = append(t.Moved, p)
	return nil
}

// Restore puts the most recent Move of p back into Files.
func (t *Trash) Restore(p string) error {
	stack := t.held[p]
	if len(stack) == 0 {
		return &fs.PathError{Op: "restore", Path: p, Err: fs.ErrNotExist}
	}
	h := stack[len(stack)-1]
	t.held[p] = stack[:len(stack)-1]
	parent := t.Files.nodes[h.parent]
	if parent == nil {
		return &fs.PathError{Op: "restore", Path: p, Err: fs.ErrNotExist}
	}
	for q, n := range h.nodes {
		t.Files.nodes[q] = n
	}
	parent.inside = append(parent.inside, h.name)
	slices.Sort(parent.inside)
	return nil
}

// View reads Files the way a trash contract watches a tree.
func (f *Files) View() TrashView { return filesView{f} }

type filesView struct{ files *Files }

func (v filesView) List(dir string) ([]scan.Entry, error) { return v.files.List(dir) }

func (v filesView) Identity(p string) (scan.FileID, error) {
	space, err := v.files.Space(p)
	if err != nil {
		return scan.FileID{}, err
	}
	return space.File, nil
}
