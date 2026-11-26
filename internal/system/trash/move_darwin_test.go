//go:build darwin

package trash

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

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
	landed := filepath.Join(home, "keep me.txt")
	if _, err := os.Lstat(landed); err != nil {
		t.Fatalf("the file is not in the Trash: %v", err)
	}
	text := onlyInfo(t, filepath.Join(home, ".info"))
	abs, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "[Trash Info]\n") || !strings.Contains(text, "Path="+escape(abs)+"\n") {
		t.Fatalf("trash info is %q, want the original path", text)
	}
	if !strings.Contains(escape(abs), "%20") {
		t.Fatalf("Path = %q, want the space percent-encoded", escape(abs))
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

func TestTheHomeVolumeUsesTheUserTrash(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	src := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := volumeTrash(home, src)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, ".Trash") {
		t.Fatalf("trash = %s, want the user's Trash", got)
	}
}

func TestTrashDirFollowsTheVolume(t *testing.T) {
	home := string(filepath.Separator) + "home"
	root := filepath.Join(string(filepath.Separator)+"volumes", "extra")
	if got := trashDir(home, 1, 1, root, 501); got != filepath.Join(home, ".Trash") {
		t.Fatalf("same volume: %s", got)
	}
	want := filepath.Join(root, ".Trashes", "501")
	if got := trashDir(home, 1, 2, root, 501); got != want {
		t.Fatalf("other volume: %s, want %s", got, want)
	}
}

func TestDarwinOpenFlagsMatchTheSyscall(t *testing.T) {
	if syscall.O_RDONLY != unix.O_RDONLY ||
		syscall.O_NOFOLLOW != unix.O_NOFOLLOW ||
		syscall.O_DIRECTORY != unix.O_DIRECTORY ||
		syscall.O_CLOEXEC != unix.O_CLOEXEC {
		t.Fatal("darwin open flags differ from syscall, so a directory open would follow a Link")
	}
}
