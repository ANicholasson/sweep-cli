package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ANicholasson/sweep-cli/internal/domain/ecosystems"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
	"github.com/ANicholasson/sweep-cli/internal/sweeptest"
	"github.com/ANicholasson/sweep-cli/internal/system/files"
	"github.com/ANicholasson/sweep-cli/internal/system/git"
)

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

func TestVersionFlagPrintsAVersionAndExitsZero(t *testing.T) {
	var stdout, stderr strings.Builder

	status := run([]string{"--version"}, &stdout, &stderr)

	if status != 0 {
		t.Errorf("exit status = %d, want 0", status)
	}
	name, version, ok := strings.Cut(strings.TrimSuffix(stdout.String(), "\n"), " ")
	if !ok || name != "sweep" || version == "" {
		t.Errorf("stdout = %q, want %q followed by a version", stdout.String(), "sweep ")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

func TestVersionFlagFailsWhenStdoutCannotBeWritten(t *testing.T) {
	var stderr strings.Builder

	status := run([]string{"--version"}, failingWriter{}, &stderr)

	if status == 0 {
		t.Errorf("exit status = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), errWriteFailed.Error()) {
		t.Errorf("stderr = %q, want it to say %q", stderr.String(), errWriteFailed)
	}
}

func TestNoArgumentsIsAUsageError(t *testing.T) {
	var stdout, stderr strings.Builder

	status := run(nil, &stdout, &stderr)

	if status != 2 {
		t.Errorf("exit status = %d, want 2", status)
	}
	if !strings.Contains(stderr.String(), "-version") {
		t.Errorf("stderr = %q, want the usage", stderr.String())
	}
}

func TestScanOffersNoFlagForDockerData(t *testing.T) {
	var stdout, stderr strings.Builder

	status := run([]string{"scan", "-h"}, &stdout, &stderr)

	if status != 2 {
		t.Errorf("exit status = %d, want 2", status)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
	if !strings.Contains(stderr.String(), "-json") {
		t.Errorf("stderr = %q, want the scan flags", stderr.String())
	}
	if strings.Contains(strings.ToLower(stderr.String()), "docker") || strings.Contains(stderr.String(), "pick") {
		t.Errorf("stderr = %q, want no flag that picks Docker data", stderr.String())
	}
}

func TestScanFailsWhenStdoutCannotBeWritten(t *testing.T) {
	var stderr strings.Builder

	status := run([]string{"scan", t.TempDir()}, failingWriter{}, &stderr)

	if status != 1 {
		t.Errorf("exit status = %d, want 1", status)
	}
	if !strings.Contains(stderr.String(), errWriteFailed.Error()) {
		t.Errorf("stderr = %q, want it to say %q", stderr.String(), errWriteFailed)
	}
}

func TestARootGivenAsALinkIsResolvedOnce(t *testing.T) {
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
	if err := os.Symlink(real, link); err != nil {
		if runtime.GOOS == "windows" && os.Getenv("CI") == "" {
			t.Skipf("this account may not make a symlink: %v", err)
		}
		t.Fatal(err)
	}
	escape := filepath.Join(real, "app", "escape")
	if err := os.Symlink(filepath.Join(outside, "pkg"), escape); err != nil {
		t.Fatal(err)
	}

	roots, err := rootsOf([]string{link})
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || !sameFile(t, string(roots[0]), real) {
		t.Fatalf("roots = %v, want %s", roots, real)
	}

	got := scan.Scan(files.Disk{}, git.Local{}, sweeptest.Docker{}, roots, ecosystems.All(), time.Now())
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
	if len(got.Unreadable) != 0 {
		t.Errorf("unreadable = %+v, want nothing", got.Unreadable)
	}
}

func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	x, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(x, y)
}
