package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ANicholasson/sweep-cli/internal/domain/ecosystems"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
	"github.com/ANicholasson/sweep-cli/internal/system/files"
	"github.com/ANicholasson/sweep-cli/internal/system/git"
)

func TestAJunctionRootIsResolvedOnce(t *testing.T) {
	top := t.TempDir()
	real := filepath.Join(top, "real")
	outside := filepath.Join(top, "outside")
	for _, p := range []string{
		filepath.Join(real, "app", "package.json"),
		filepath.Join(real, "app", "node_modules", "a", "index.js"),
		filepath.Join(outside, "pkg", "package.json"),
		filepath.Join(outside, "pkg", "node_modules", "b", "index.js"),
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(top, "through")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, real).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J: %v\n%s", err, out)
	}
	escape := filepath.Join(real, "app", "escape")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", escape, filepath.Join(outside, "pkg")).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J: %v\n%s", err, out)
	}

	roots, err := rootsOf([]string{link})
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || !sameFile(t, string(roots[0]), real) {
		t.Fatalf("roots = %v, want %s", roots, real)
	}

	got := scan.Scan(files.Disk{}, git.Local{}, sweeptest.Docker{}, roots, ecosystems.All())
	want := filepath.Join(real, "app", "node_modules")
	if len(got.Candidates) != 1 || !sameFile(t, got.Candidates[0].Path, want) {
		t.Errorf("candidates = %+v, want %s", got.Candidates, want)
	}
	escaped := filepath.Join(escape, "node_modules")
	for _, c := range got.Candidates {
		if sameFile(t, c.Path, escaped) {
			t.Errorf("listed %s, which is through a Link", c.Path)
		}
	}
}
