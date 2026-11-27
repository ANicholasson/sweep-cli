//go:build linux

package trash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	text := onlyInfo(t, filepath.Join(home, "info"))
	abs, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "[Trash Info]\n") || !strings.Contains(text, "Path="+escape(abs)+"\n") {
		t.Fatalf("trash info is %q, want the original path", text)
	}
	if !strings.Contains(text, "Path="+escape(abs)) || !strings.Contains(escape(abs), "%20") {
		t.Fatalf("Path = %q, want the space percent-encoded", escape(abs))
	}
	landed := filepath.Join(home, "files", "keep me.txt")
	if _, err := os.Lstat(landed); err != nil {
		t.Fatalf("the file is not in the trash: %v", err)
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
