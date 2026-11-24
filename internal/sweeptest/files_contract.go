package sweeptest

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// FilesContract is what a Scan relies on from a scan.Files.
//
// open builds the Tree and returns the Files that reads it and the path of
// its top.
func FilesContract(t *testing.T, open func(t *testing.T, tree Tree) (files scan.Files, top string)) {
	t.Run("List names what is directly inside, in order, and says what each is", func(t *testing.T) {
		files, top := open(t, Tree{
			"b.txt":       File{Bytes: 1},
			"a/x.txt":     File{Bytes: 1},
			"a/deep/y.js": File{Bytes: 1},
			"empty":       Dir{},
			"c":           Link{To: "a"},
		})
		want := []string{"a Dir", "b.txt File", "c Link", "empty Dir"}
		if got := names(list(t, files, top)); !slices.Equal(got, want) {
			t.Errorf("List = %v, want %v", got, want)
		}
	})

	t.Run("an Entry's Path lists what is inside it", func(t *testing.T) {
		files, top := open(t, Tree{"a/deep/y.js": File{Bytes: 1}})
		deep := find(t, files, top, "a/deep")
		if got := names(list(t, files, deep.Path)); !slices.Equal(got, []string{"y.js File"}) {
			t.Errorf("List of a/deep = %v, want [y.js File]", got)
		}
	})

	t.Run("a Link is a Link whatever it leads to", func(t *testing.T) {
		files, top := open(t, Tree{
			"dir/f.txt": File{Bytes: 1},
			"to-dir":    Link{To: "dir"},
			"to-file":   Link{To: "dir/f.txt"},
			"to-nought": Link{To: "gone"},
		})
		for _, name := range []string{"to-dir", "to-file", "to-nought"} {
			if e := find(t, files, top, name); e.Is != scan.Link {
				t.Errorf("%s is %v, want a Link", name, e.Is)
			}
		}
	})

	t.Run("List fails on a file and on a path that is not there", func(t *testing.T) {
		files, top := open(t, Tree{"f.txt": File{Bytes: 1}})
		if _, err := files.List(find(t, files, top, "f.txt").Path); err == nil {
			t.Error("List of a file gave no error")
		}
		if _, err := files.List(top + "-gone"); err == nil {
			t.Error("List of a missing path gave no error")
		}
	})

	t.Run("every file and directory has a FileID of its own, the same each time", func(t *testing.T) {
		files, top := open(t, Tree{
			"a/x.txt": File{Bytes: 1},
			"a/z":     Dir{},
			"b/y.txt": File{Bytes: 1},
		})
		owner := map[scan.FileID]string{}
		for _, p := range []string{"a", "a/x.txt", "a/z", "b", "b/y.txt"} {
			id := space(t, files, find(t, files, top, p)).File
			if id == (scan.FileID{}) {
				t.Errorf("%s has the zero FileID", p)
			}
			if other, ok := owner[id]; ok {
				t.Errorf("%s and %s share FileID %+v", other, p, id)
			}
			owner[id] = p
			if again := space(t, files, find(t, files, top, p)).File; again != id {
				t.Errorf("%s has FileID %+v, then %+v", p, id, again)
			}
		}
	})

	t.Run("hard links to one file share their Space", func(t *testing.T) {
		files, top := open(t, Tree{
			"one/LICENSE": File{Bytes: 64 << 10},
			"two/LICENSE": HardLink{To: "one/LICENSE"},
		})
		first := space(t, files, find(t, files, top, "one/LICENSE"))
		second := space(t, files, find(t, files, top, "two/LICENSE"))
		if first != second {
			t.Errorf("two hard links to one file have Space %+v and %+v, want the same", first, second)
		}
	})

	t.Run("Space covers a file's bytes in whole 512-byte blocks", func(t *testing.T) {
		files, top := open(t, Tree{"big": File{Bytes: 64 << 10}})
		got := space(t, files, find(t, files, top, "big"))
		if got.Bytes < 64<<10 || got.Bytes%512 != 0 {
			t.Errorf("Space of a 64 KiB file = %d bytes, want at least %d in whole 512-byte blocks", got.Bytes, 64<<10)
		}
	})

	t.Run("Space is the path's own, not what a directory holds or a Link leads to", func(t *testing.T) {
		files, top := open(t, Tree{
			"dir/big": File{Bytes: 256 << 10},
			"to-big":  Link{To: "dir/big"},
		})
		big := space(t, files, find(t, files, top, "dir/big"))
		for _, name := range []string{"dir", "to-big"} {
			got := space(t, files, find(t, files, top, name))
			if got.Bytes >= 256<<10 {
				t.Errorf("Space of %s = %d bytes, which takes in the 256 KiB file", name, got.Bytes)
			}
			if got.File == big.File {
				t.Errorf("%s has the FileID of the file", name)
			}
		}
	})

	t.Run("Space fails on a path that is not there", func(t *testing.T) {
		files, top := open(t, Tree{})
		if _, err := files.Space(top + "-gone"); err == nil {
			t.Error("Space of a missing path gave no error")
		}
	})

	t.Run("Modified is the modification time, and a missing path fails", func(t *testing.T) {
		when := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
		files, top := open(t, Tree{"a/old.js": File{Bytes: 1, Modified: when}})
		got := modified(t, files, find(t, files, top, "a/old.js"))
		if !got.Equal(when) {
			t.Errorf("Modified = %s, want %s", got, when)
		}
		if _, err := files.Modified(top + "-gone"); err == nil {
			t.Error("Modified of a missing path gave no error")
		}
	})

	t.Run("a path longer than 260 characters lists and measures", func(t *testing.T) {
		long := strings.Repeat("node_modules/a-package-with-a-long-name/", 8) + "index.js"
		files, top := open(t, Tree{long: File{Bytes: 1}})
		space(t, files, find(t, files, top, long))
	})
}

func list(t *testing.T, files scan.Files, dir string) []scan.Entry {
	t.Helper()
	entries, err := files.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func space(t *testing.T, files scan.Files, e scan.Entry) scan.Space {
	t.Helper()
	s, err := files.Space(e.Path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func modified(t *testing.T, files scan.Files, e scan.Entry) time.Time {
	t.Helper()
	got, err := files.Modified(e.Path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func find(t *testing.T, files scan.Files, top, p string) scan.Entry {
	t.Helper()
	dir := top
	var found scan.Entry
	for _, name := range strings.Split(p, "/") {
		entries := list(t, files, dir)
		i := slices.IndexFunc(entries, func(e scan.Entry) bool { return e.Name == name })
		if i < 0 {
			t.Fatalf("%s is not in %s", name, dir)
		}
		found, dir = entries[i], entries[i].Path
	}
	return found
}

func names(entries []scan.Entry) []string {
	what := map[scan.Is]string{scan.File: "File", scan.Dir: "Dir", scan.Link: "Link", scan.Other: "Other"}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name + " " + what[e.Is]
	}
	return out
}
