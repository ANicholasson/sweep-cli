//go:build windows

package trash

import (
	"cmp"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/clean"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestTheWindowsTrashPassesTheContract(t *testing.T) {
	sweeptest.TrashContract(t, func(t *testing.T, tree sweeptest.Tree) (clean.Trash, sweeptest.TrashView, string) {
		top := sweeptest.OnDisk(t, tree)
		// The trash directory is one this test created. It is not the
		// machine's Recycle Bin.
		return In(filepath.Join(t.TempDir(), "Trash")), diskView{}, top
	})
}

func TestTheTrashInfoNamesTheOriginalPath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "keep me.txt")
	if err := os.WriteFile(src, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "Trash")
	if err := In(home).Move(dir, src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Fatalf("the file is still at %s: %v", src, err)
	}
	text := onlyInfo(t, home)
	abs, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "[Trash Info]\n") || !strings.Contains(text, "Path="+escape(abs)+"\n") {
		t.Fatalf("trash info is %q, want the original path", text)
	}
	if !strings.Contains(escape(abs), "%20") || !strings.Contains(escape(abs), "%5C") {
		t.Fatalf("Path = %q, want the space and the separators percent-encoded", escape(abs))
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
	linkTo(t, real, link)
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
	linkTo(t, real, filepath.Join(dir, "via"))
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
	linkTo(t, filepath.Join(dir, "real"), filepath.Join(dir, "fx"))
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

func TestALongPathMovesIntoAPrivateTrash(t *testing.T) {
	dir := t.TempDir()
	cur := dir
	for len(cur) < 280 {
		cur = filepath.Join(cur, "nested-name")
	}
	if err := os.MkdirAll(cur, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(cur, "keep.txt")
	if len(src) <= 260 {
		t.Fatalf("path length %d, want a path past 260 characters", len(src))
	}
	if err := os.WriteFile(src, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := mustID(t, src)
	home := filepath.Join(dir, "Trash")
	if err := In(home).Move(dir, src); err != nil {
		still, readErr := os.ReadFile(src)
		t.Fatalf("Move of a long path: %v; the file is %q, %v", err, still, readErr)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Fatalf("the long path is still in place: %v", err)
	}
	if err := In(home).Restore(dir, src); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(src)
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("restored %q, %v", got, err)
	}
	if mustID(t, src) != before {
		t.Errorf("restore changed the file identity")
	}
}

func TestRestorePutsTheOriginalAttributesBack(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	dir := t.TempDir()
	src := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(src, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	current, err := fileAttributes(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := setAttributes(src, current|syscall.FILE_ATTRIBUTE_READONLY); err != nil {
		t.Fatal(err)
	}
	before, err := fileAttributes(src)
	if err != nil {
		t.Fatal(err)
	}
	if before&syscall.FILE_ATTRIBUTE_READONLY == 0 || before&syscall.FILE_ATTRIBUTE_HIDDEN != 0 {
		t.Fatalf("attributes = %#x, want read-only and visible", before)
	}
	putBack(t, dir, src)
	abs, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := bin(t).Move(dir, src); err != nil {
		t.Fatal(err)
	}
	home, err := recycleBin(abs)
	if err != nil {
		t.Fatal(err)
	}
	held, err := fileAttributes(filepath.Join(home, "$R"+recycledSuffix(t, home, abs)))
	if err != nil {
		t.Fatal(err)
	}
	if held&syscall.FILE_ATTRIBUTE_READONLY == 0 || held == syscall.FILE_ATTRIBUTE_HIDDEN {
		t.Errorf("attributes in the Recycle Bin = %#x, want the original %#x", held, before)
	}
	if err := bin(t).Restore(dir, src); err != nil {
		t.Fatal(err)
	}
	after, err := fileAttributes(src)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("attributes after restore = %#x, want %#x", after, before)
	}
	got, err := os.ReadFile(src)
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("restored %q, %v", got, err)
	}
}

func TestATrashedFileAppearsInTheRecycleBin(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	dir := t.TempDir()
	src := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(src, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	putBack(t, dir, src)
	before := mustID(t, src)
	abs, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := bin(t).Move(dir, src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Fatalf("the file is still at %s: %v", src, err)
	}
	home, err := recycleBin(abs)
	if err != nil {
		t.Fatal(err)
	}
	suffix := recycledSuffix(t, home, abs)
	landed := filepath.Join(home, "$R"+suffix)
	if mustID(t, landed) != before {
		t.Errorf("the Recycle Bin holds a different file")
	}
	info, err := os.ReadFile(filepath.Join(home, "$I"+suffix))
	if err != nil {
		t.Fatal(err)
	}
	gotPath, _, _, ok := parseRecycleInfo(info)
	if !ok || gotPath != abs {
		t.Fatalf("recycle info path = %q, ok %v, want %s", gotPath, ok, abs)
	}
	if err := bin(t).Restore(dir, src); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(src)
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("restored %q, %v", got, err)
	}
	if mustID(t, src) != before {
		t.Errorf("restore changed the file identity")
	}
	if _, err := os.Lstat(landed); !os.IsNotExist(err) {
		t.Errorf("the Recycle Bin still holds the file: %v", err)
	}
}

func TestTwoFilesOfOneNameBothRestoreFromTheRecycleBin(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	dir := t.TempDir()
	a := filepath.Join(dir, "a", "note.txt")
	b := filepath.Join(dir, "b", "note.txt")
	if err := os.MkdirAll(filepath.Dir(a), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(b), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	putBack(t, dir, a)
	putBack(t, dir, b)
	aID, bID := mustID(t, a), mustID(t, b)
	if err := bin(t).Move(dir, a); err != nil {
		t.Fatal(err)
	}
	if err := bin(t).Move(dir, b); err != nil {
		t.Fatal(err)
	}
	if err := bin(t).Restore(dir, a); err != nil {
		t.Fatal(err)
	}
	if err := bin(t).Restore(dir, b); err != nil {
		t.Fatal(err)
	}
	gotA, err := os.ReadFile(a)
	if err != nil || string(gotA) != "a\n" || mustID(t, a) != aID {
		t.Fatalf("a restored as %q, %v", gotA, err)
	}
	gotB, err := os.ReadFile(b)
	if err != nil || string(gotB) != "b\n" || mustID(t, b) != bID {
		t.Fatalf("b restored as %q, %v", gotB, err)
	}
}

func TestATrashedDirectoryAppearsInTheRecycleBin(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	dir := t.TempDir()
	folder := filepath.Join(dir, "dir")
	child := filepath.Join(folder, "child.txt")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte("child\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	putBack(t, dir, folder)
	before := mustID(t, folder)
	childBefore := mustID(t, child)
	if err := bin(t).Move(dir, folder); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(folder); !os.IsNotExist(err) {
		t.Fatalf("the directory is still in place: %v", err)
	}
	if err := bin(t).Restore(dir, folder); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(child)
	if err != nil || string(got) != "child\n" {
		t.Fatalf("restored child %q, %v", got, err)
	}
	if mustID(t, folder) != before || mustID(t, child) != childBefore {
		t.Errorf("restore changed the file identity")
	}
}

func TestAFileTheRecycleBinCannotTakeStays(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	dir := t.TempDir()
	src := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(src, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := mustID(t, src)
	held := holdExclusive(t, src)
	defer func() { _ = syscall.CloseHandle(held) }()
	err := bin(t).Move(dir, src)
	if err == nil {
		t.Fatal("Move of a file that cannot be renamed returned no error")
	}
	got, err := os.ReadFile(src)
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("the file is %q, %v, want it left in place", got, err)
	}
	if mustID(t, src) != before {
		t.Errorf("the file was replaced")
	}
}

func TestALongPathIsTrashedOrLeftInPlace(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	dir := t.TempDir()
	cur := dir
	for len(cur) < 280 {
		cur = filepath.Join(cur, "nested-name")
	}
	if err := os.MkdirAll(cur, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(cur, "keep.txt")
	if len(src) <= 260 {
		t.Fatalf("path length %d, want a path past 260 characters", len(src))
	}
	if err := os.WriteFile(src, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	putBack(t, dir, src)
	before := mustID(t, src)
	err := bin(t).Move(dir, src)
	if err != nil {
		got, readErr := os.ReadFile(src)
		if readErr != nil || string(got) != "keep\n" || mustID(t, src) != before {
			t.Fatalf("a failed Move of a long path removed it: %v; file %q, %v", err, got, readErr)
		}
		return
	}
	if _, statErr := os.Lstat(src); !os.IsNotExist(statErr) {
		t.Fatalf("Move reported success and left the file: %v", statErr)
	}
	if err := bin(t).Restore(dir, src); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(src)
	if err != nil || string(got) != "keep\n" || mustID(t, src) != before {
		t.Fatalf("restored %q, %v", got, err)
	}
}

func TestAPathOnAnotherDriveIsTrashedOrLeftInPlace(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	letter := freeDrive(t)
	dir := t.TempDir()
	if out, err := exec.Command("subst", letter+":", dir).CombinedOutput(); err != nil {
		if os.Getenv("CI") == "" {
			t.Skipf("subst %s: %v: %s", letter, err, out)
		}
		t.Fatalf("subst %s: %v: %s", letter, err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("subst", letter+":", "/d").Run()
	})
	src := letter + `:\keep.txt`
	if err := os.WriteFile(src, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	putBack(t, letter+`:\`, src)
	before := mustID(t, src)
	err := bin(t).Move(letter+`:\`, src)
	if err != nil {
		got, readErr := os.ReadFile(src)
		if readErr != nil || string(got) != "keep\n" || mustID(t, src) != before {
			t.Fatalf("a failed Move on %s: removed the file: %v; file %q, %v", letter, err, got, readErr)
		}
		return
	}
	if _, statErr := os.Lstat(src); !os.IsNotExist(statErr) {
		t.Fatalf("Move reported success and left the file: %v", statErr)
	}
	if err := bin(t).Restore(letter+`:\`, src); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(src)
	if err != nil || string(got) != "keep\n" || mustID(t, src) != before {
		t.Fatalf("restored %q, %v", got, err)
	}
}

func bin(t *testing.T) Mover {
	t.Helper()
	mover, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if mover.dir != "" {
		t.Fatal("Open used a private directory; the Recycle Bin was not asked")
	}
	return mover
}

func putBack(t *testing.T, root, path string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return
		}
		mover, err := Open()
		if err != nil {
			return
		}
		_ = mover.Restore(root, path)
	})
}

func recycledSuffix(t *testing.T, binDir, abs string) string {
	t.Helper()
	suffix, _, err := findRecycled(binDir, abs)
	if err != nil {
		t.Fatal(err)
	}
	return suffix
}

func holdExclusive(t *testing.T, path string) syscall.Handle {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(long(path))
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func freeDrive(t *testing.T) string {
	t.Helper()
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetLogicalDrives")
	mask, _, _ := proc.Call()
	if mask == 0 {
		t.Fatal("GetLogicalDrives returned no drives")
	}
	for letter := byte('Z'); letter >= 'D'; letter-- {
		if mask&(1<<uint(letter-'A')) == 0 {
			return string(letter)
		}
	}
	t.Skip("no free drive letter")
	return ""
}

func onlyInfo(t *testing.T, home string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(home, "info"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("%d info files, want one", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(home, "info", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func linkTo(t *testing.T, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err == nil {
		return
	}
	out, jerr := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if jerr != nil {
		if os.Getenv("CI") == "" {
			t.Skipf("this account may not make a Link: %v", err)
		}
		t.Fatalf("mklink /J: %v: %s", jerr, out)
	}
}

func mustID(t *testing.T, path string) scan.FileID {
	t.Helper()
	id, err := (diskView{}).Identity(path)
	if err != nil {
		t.Fatal(err)
	}
	if id == (scan.FileID{}) {
		t.Fatalf("%s has the zero FileID", path)
	}
	return id
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

func (diskView) Identity(p string) (scan.FileID, error) {
	name, err := syscall.UTF16PtrFromString(long(p))
	if err != nil {
		return scan.FileID{}, err
	}
	const share = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE
	const flags = syscall.FILE_FLAG_BACKUP_SEMANTICS | syscall.FILE_FLAG_OPEN_REPARSE_POINT
	h, err := syscall.CreateFile(name, 0, share, nil, syscall.OPEN_EXISTING, flags, 0)
	if err != nil {
		return scan.FileID{}, err
	}
	defer func() { _ = syscall.CloseHandle(h) }()
	info, err := handleInfo(h)
	if err != nil {
		return scan.FileID{}, err
	}
	return scan.FileID{
		Volume: uint64(info.VolumeSerialNumber),
		File:   uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow),
	}, nil
}
