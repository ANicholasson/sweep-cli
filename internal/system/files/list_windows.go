package files

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// openDir opens dir without following a Link at the end of the path. A
// directory swapped for a Link, including a junction, between the listing
// that named it and this open is not followed.
func openDir(dir string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(long(dir))
	if err != nil {
		return nil, &fs.PathError{Op: "list", Path: dir, Err: err}
	}
	const share = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE
	const flags = syscall.FILE_FLAG_BACKUP_SEMANTICS | syscall.FILE_FLAG_OPEN_REPARSE_POINT
	access := uint32(syscall.FILE_LIST_DIRECTORY | syscall.SYNCHRONIZE)
	h, err := syscall.CreateFile(name, access, share, nil, syscall.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, &fs.PathError{Op: "list", Path: dir, Err: err}
	}
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		_ = syscall.CloseHandle(h)
		return nil, &fs.PathError{Op: "list", Path: dir, Err: err}
	}
	switch {
	case info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0:
		_ = syscall.CloseHandle(h)
		return nil, &fs.PathError{Op: "list", Path: dir, Err: errors.New("it is a Link")}
	case info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == 0:
		_ = syscall.CloseHandle(h)
		return nil, &fs.PathError{Op: "list", Path: dir, Err: syscall.ENOTDIR}
	}
	return os.NewFile(uintptr(h), dir), nil
}
