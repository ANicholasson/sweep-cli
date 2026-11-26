package sweeptest

import (
	"slices"
	"strings"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/clean"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// TrashView is how a trash contract watches the tree a Trash moves.
type TrashView interface {
	List(dir string) ([]scan.Entry, error)
	Identity(path string) (scan.FileID, error)
}

// TrashContract is what a Clean relies on from a clean.Trash.
//
// open builds the Tree and returns the Trash that moves it, a view of that
// tree, and the path of its top. The trash directory the adapter uses is
// one open created, never the machine's trash.
func TrashContract(t *testing.T, open func(t *testing.T, tree Tree) (clean.Trash, TrashView, string)) {
	t.Run("a file moves into the trash and restores as the same file", func(t *testing.T) {
		bin, view, top := open(t, Tree{"keep.txt": File{Bytes: 4}})
		file := named(t, view, top, "keep.txt")
		id := identity(t, view, file)

		if err := bin.Move(file); err != nil {
			t.Fatal(err)
		}
		if slices.Contains(entryNames(t, view, top), "keep.txt") {
			t.Fatal("the file is still there")
		}
		if err := bin.Restore(file); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(entryNames(t, view, top), "keep.txt") {
			t.Fatal("the file was not restored")
		}
		if got := identity(t, view, file); got != id {
			t.Errorf("identity after restore = %+v, want %+v", got, id)
		}
	})

	t.Run("a directory moves with what is inside it and restores", func(t *testing.T) {
		bin, view, top := open(t, Tree{"dir/child.txt": File{Bytes: 4}})
		dir := named(t, view, top, "dir")
		child := named(t, view, dir, "child.txt")
		dirID := identity(t, view, dir)
		childID := identity(t, view, child)

		if err := bin.Move(dir); err != nil {
			t.Fatal(err)
		}
		if slices.Contains(entryNames(t, view, top), "dir") {
			t.Fatal("the directory is still there")
		}
		if _, err := view.List(dir); err == nil {
			t.Fatal("the moved directory can still be listed")
		}
		if err := bin.Restore(dir); err != nil {
			t.Fatal(err)
		}
		if identity(t, view, dir) != dirID || identity(t, view, child) != childID {
			t.Errorf("restore changed the file identity")
		}
		if !slices.Contains(entryNames(t, view, dir), "child.txt") {
			t.Fatal("the directory came back without what was inside it")
		}
	})

	t.Run("a missing path is not moved", func(t *testing.T) {
		bin, view, top := open(t, Tree{"keep.txt": File{Bytes: 1}})
		before := entryNames(t, view, top)
		missing := top + "/does-not-exist"
		if strings.Contains(top, `\`) {
			missing = top + `\does-not-exist`
		}
		if err := bin.Move(missing); err == nil {
			t.Fatal("Move of a missing path returned no error")
		}
		if got := entryNames(t, view, top); !slices.Equal(got, before) {
			t.Errorf("List = %v, want %v", got, before)
		}
	})

	t.Run("two files of one name both restore", func(t *testing.T) {
		bin, view, top := open(t, Tree{
			"a/note.txt": File{Bytes: 1},
			"b/note.txt": File{Bytes: 2},
		})
		a := named(t, view, named(t, view, top, "a"), "note.txt")
		b := named(t, view, named(t, view, top, "b"), "note.txt")
		aID := identity(t, view, a)
		bID := identity(t, view, b)

		if err := bin.Move(a); err != nil {
			t.Fatal(err)
		}
		if err := bin.Move(b); err != nil {
			t.Fatal(err)
		}
		if err := bin.Restore(a); err != nil {
			t.Fatal(err)
		}
		if err := bin.Restore(b); err != nil {
			t.Fatal(err)
		}
		if identity(t, view, a) != aID || identity(t, view, b) != bID {
			t.Errorf("the two files were not restored as themselves")
		}
	})
}

func named(t *testing.T, view TrashView, dir, name string) string {
	t.Helper()
	entries, err := view.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == name {
			return e.Path
		}
	}
	t.Fatalf("%s is not in %s", name, dir)
	return ""
}

func entryNames(t *testing.T, view TrashView, dir string) []string {
	t.Helper()
	entries, err := view.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func identity(t *testing.T, view TrashView, p string) scan.FileID {
	t.Helper()
	id, err := view.Identity(p)
	if err != nil {
		t.Fatal(err)
	}
	if id == (scan.FileID{}) {
		t.Fatalf("%s has the zero FileID", p)
	}
	return id
}
