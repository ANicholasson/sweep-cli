//go:build linux || darwin

package trash

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/clean"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestTheTrashPassesTheContract(t *testing.T) {
	sweeptest.TrashContract(t, func(t *testing.T, tree sweeptest.Tree) (clean.Trash, sweeptest.TrashView, string) {
		top := sweeptest.OnDisk(t, tree)
		return In(filepath.Join(t.TempDir(), "Trash")), diskView{}, top
	})
}

func TestAFailedMoveLeavesTheFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(src, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := In(filepath.Join(blocker, "Trash")).Move(dir, src)
	if err == nil {
		t.Fatal("Move returned no error, and it must not delete the file instead")
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Errorf("the file is %q, want it left in place", got)
	}
}

func TestALinkAboveTheTrashStillReceivesTheFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(src, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(link, "Trash")
	err := In(home).Move(dir, src)
	if err != nil {
		if strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("a Link above the trash was refused as not a directory: %v", err)
		}
		t.Fatal(err)
	}
	if _, statErr := os.Lstat(src); !os.IsNotExist(statErr) {
		t.Fatalf("the file is still at %s: %v", src, statErr)
	}
	if err := In(home).Restore(dir, src); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Errorf("restored %q, want the original bytes", got)
	}
}

func TestMoveDoesNotFollowALinkAboveTheFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	precious := filepath.Join(real, "precious.txt")
	if err := os.WriteFile(precious, []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "via")); err != nil {
		t.Fatal(err)
	}
	err := In(filepath.Join(dir, "Trash")).Move(dir, filepath.Join(dir, "via", "precious.txt"))
	if err == nil {
		t.Fatal("Move followed a Link")
	}
	got, err := os.ReadFile(precious)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "precious\n" {
		t.Errorf("the file the Link leads to is %q", got)
	}
}

func TestALinkAboveTheRootIsFollowed(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real", "projects", "web-app")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(real, "keep.txt")
	if err := os.WriteFile(src, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "fx")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "fx", "projects")
	spelled := filepath.Join(root, "web-app", "keep.txt")
	home := filepath.Join(dir, "Trash")
	if err := In(home).Move(root, spelled); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Fatalf("the file is still at %s: %v", src, err)
	}
	if err := In(home).Restore(root, spelled); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Errorf("restored %q, want the original bytes", got)
	}
}

type diskView struct{}

func (diskView) List(dir string) ([]scan.Entry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return cmp.Compare(a.Name(), b.Name()) })
	out := make([]scan.Entry, len(entries))
	for i, e := range entries {
		kind := scan.Other
		switch {
		case e.Type()&os.ModeSymlink != 0:
			kind = scan.Link
		case e.IsDir():
			kind = scan.Dir
		case e.Type().IsRegular():
			kind = scan.File
		}
		out[i] = scan.Entry{Name: e.Name(), Path: filepath.Join(dir, e.Name()), Is: kind}
	}
	return out, nil
}

func onlyInfo(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("%d info files, want one", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (diskView) Identity(p string) (scan.FileID, error) {
	info, err := os.Lstat(p)
	if err != nil {
		return scan.FileID{}, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return scan.FileID{}, errors.New("the file system gave no identity")
	}
	return scan.FileID{Volume: uint64(st.Dev), File: uint64(st.Ino)}, nil
}
