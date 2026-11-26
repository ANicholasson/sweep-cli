//go:build windows

package main_test

import (
	"strings"
	"syscall"
	"testing"
)

func diskID(t *testing.T, path string) (uint64, uint64) {
	t.Helper()
	if len(path) >= 248 && !strings.HasPrefix(path, `\\?\`) {
		if strings.HasPrefix(path, `\\`) {
			path = `\\?\UNC\` + path[2:]
		} else {
			path = `\\?\` + path
		}
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	const share = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE
	const itself = syscall.FILE_FLAG_BACKUP_SEMANTICS | syscall.FILE_FLAG_OPEN_REPARSE_POINT
	h, err := syscall.CreateFile(name, 0, share, nil, syscall.OPEN_EXISTING, itself, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.CloseHandle(h) }()
	var id syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &id); err != nil {
		t.Fatal(err)
	}
	return uint64(id.VolumeSerialNumber), uint64(id.FileIndexHigh)<<32 | uint64(id.FileIndexLow)
}
