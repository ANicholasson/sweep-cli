//go:build unix

package main

import (
	"bytes"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
)

func TestAFailedWriteLeavesNoTempFile(t *testing.T) {
	// The built binary is killed by the file-size signal before it can
	// return the write error, so this calls writePlan in this process,
	// with that signal ignored.
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &old)
	})
	signal.Ignore(syscall.SIGXFSZ)
	limited := old
	limited.Cur = 0
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limited); err != nil {
		t.Fatal(err)
	}

	err := writePlan(planPath, bytes.Repeat([]byte("x"), 64))
	if !errors.Is(err, syscall.EFBIG) {
		t.Fatalf("writePlan error = %v, want the file size limit", err)
	}
	if _, err := os.Lstat(planPath); !os.IsNotExist(err) {
		t.Errorf("the Plan path was created: %v", err)
	}
	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("a failed write left %v", left)
	}
}
