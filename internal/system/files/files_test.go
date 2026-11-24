package files

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
)

func TestResolveLeavesADirectoryAndFollowsAChainOfLinks(t *testing.T) {
	dir := t.TempDir()
	got, err := Resolve(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("Resolve of a directory = %q, want %q", got, dir)
	}

	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	mid := filepath.Join(dir, "mid")
	end := filepath.Join(dir, "end")
	if err := os.Symlink("real", mid); err != nil {
		if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
			t.Skipf("this account may not make a symlink: %v", err)
		}
		t.Fatal(err)
	}
	if err := os.Symlink("mid", end); err != nil {
		t.Fatal(err)
	}
	got, err = Resolve(end)
	if err != nil {
		t.Fatal(err)
	}
	if got != real {
		t.Errorf("Resolve of a chain = %q, want %q", got, real)
	}
	info, err := os.Lstat(got)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("Resolve returned a Link %s", got)
	}
}

func TestResolveFollowsALinkBeforeDotDot(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "actual", "selected")
	decoy := filepath.Join(dir, "selected")
	for _, p := range []string{filepath.Join(want, "keep"), filepath.Join(dir, "actual", "nested"), decoy} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("actual/nested", filepath.Join(dir, "mid")); err != nil {
		if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
			t.Skipf("this account may not make a symlink: %v", err)
		}
		t.Fatal(err)
	}
	// The target is stored with ".." still in it. Join would remove ".."
	// before the Link at mid is followed.
	link := filepath.Join(dir, "rootlink")
	if err := os.Symlink("mid/../selected", link); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(link)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Resolve = %s, want %s", got, want)
	}
}

func TestDiskPassesTheFilesContract(t *testing.T) {
	sweeptest.FilesContract(t, func(t *testing.T, tree sweeptest.Tree) (scan.Files, string) {
		return Disk{}, sweeptest.OnDisk(t, tree)
	})
}
