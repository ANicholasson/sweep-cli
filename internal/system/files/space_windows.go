package files

import (
	"io/fs"
	"strings"
	"syscall"
	"unsafe"

	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// The syscall package has GetFileInformationByHandle, which gives a file's
// identity, but not the call that gives its allocation size. One lazy
// procedure from kernel32 gets it, so sweep stays on the standard library.
var getFileInformationByHandleEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFileInformationByHandleEx")

// fileStandardInfo is FILE_STANDARD_INFO, class 1 of
// GetFileInformationByHandleEx.
type fileStandardInfo struct {
	AllocationSize int64
	EndOfFile      int64
	NumberOfLinks  uint32
	DeletePending  byte
	Directory      byte
}

const fileStandardInfoClass = 1

// Space opens the path itself, a Link included, with no access but to its
// attributes, and asks Windows for its identity and its allocation size.
//
// Windows reports no allocation for a small file whose bytes live in its
// file record, and then the bytes it holds stand in. The result is rounded
// up to 1024, because the Cygwin runtime under Git Bash counts st_blocks in
// 1024-byte blocks with the same stand-in, so the du of Git Bash, the
// independent source the end-to-end test compares a Size with, counts the
// same way.
func (Disk) Space(path string) (scan.Space, error) {
	name, err := syscall.UTF16PtrFromString(long(path))
	if err != nil {
		return scan.Space{}, &fs.PathError{Op: "space", Path: path, Err: err}
	}
	const share = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE
	const itself = syscall.FILE_FLAG_BACKUP_SEMANTICS | syscall.FILE_FLAG_OPEN_REPARSE_POINT
	h, err := syscall.CreateFile(name, 0, share, nil, syscall.OPEN_EXISTING, itself, 0)
	if err != nil {
		return scan.Space{}, &fs.PathError{Op: "space", Path: path, Err: err}
	}
	defer func() { _ = syscall.CloseHandle(h) }()

	var id syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &id); err != nil {
		return scan.Space{}, &fs.PathError{Op: "space", Path: path, Err: err}
	}
	var std fileStandardInfo
	ok, _, err := getFileInformationByHandleEx.Call(uintptr(h), fileStandardInfoClass, uintptr(unsafe.Pointer(&std)), unsafe.Sizeof(std))
	if ok == 0 {
		return scan.Space{}, &fs.PathError{Op: "space", Path: path, Err: err}
	}

	allocated := std.AllocationSize
	if allocated == 0 && std.Directory == 0 {
		allocated = std.EndOfFile
	}
	return scan.Space{
		Bytes: (allocated + 1023) / 1024 * 1024,
		File: scan.FileID{
			Volume: uint64(id.VolumeSerialNumber),
			File:   uint64(id.FileIndexHigh)<<32 | uint64(id.FileIndexLow),
		},
	}, nil
}

// long gives a path the prefix that lifts the 260-character limit, which a
// path deep inside a dependency directory often passes. The os package does
// this for itself, and syscall.CreateFile does not.
func long(path string) string {
	switch {
	case len(path) < 248 || strings.HasPrefix(path, `\\?\`):
		return path
	case strings.HasPrefix(path, `\\`):
		return `\\?\UNC\` + path[2:]
	default:
		return `\\?\` + path
	}
}
