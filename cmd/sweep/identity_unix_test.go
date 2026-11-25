//go:build unix

package main_test

import (
	"os"
	"syscall"
	"testing"
)

func diskID(t *testing.T, path string) (uint64, uint64) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("%s has no file identity", path)
	}
	return uint64(st.Dev), uint64(st.Ino)
}
