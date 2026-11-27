//go:build windows

package trash

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// Delete removes path. root is the Root path was reached from. A Link above
// root is followed, because that is how the caller spelled it. A Link at
// root or beneath it is not followed. A Link at path or inside it is
// removed as a Link and its target stays. A missing path is an error.
func (Mover) Delete(root, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	parent, err := openBeneath(filepath.Dir(abs), root, parentAccess)
	if err != nil {
		return err
	}
	defer func() { _ = syscall.CloseHandle(parent) }()
	return removeRelative(parent, filepath.Base(abs))
}

func removeRelative(parent syscall.Handle, name string) error {
	if name == "" || name == "." || name == ".." {
		return &os.PathError{Op: "delete", Path: name, Err: errors.New("a component has no usable name")}
	}
	h, err := openRelative(parent, name, deleteAccess|fileListDirectory|fileTraverse|fileReadAttributes|synchronize|standardRightsRead, childOptions)
	if err != nil {
		return err
	}
	defer func() { _ = syscall.CloseHandle(h) }()
	rep, err := handleIsReparse(h)
	if err != nil {
		return err
	}
	if !rep {
		dir, err := handleIsDir(h)
		if err != nil {
			return err
		}
		if dir {
			names, err := listNames(h)
			if err != nil {
				return err
			}
			for _, n := range names {
				if err := removeRelative(h, n); err != nil {
					return err
				}
			}
		}
	}
	return markDeleted(h)
}

var queryDirectory = ntdll.NewProc("NtQueryDirectoryFile")

func listNames(h syscall.Handle) ([]string, error) {
	buf := make([]byte, 64*1024)
	var names []string
	restart := uintptr(1)
	for {
		var iosb ioStatusBlock
		status, _, _ := queryDirectory.Call(
			uintptr(h),
			0, 0, 0,
			uintptr(unsafe.Pointer(&iosb)),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			1,
			1,
			0,
			restart,
		)
		runtime.KeepAlive(&iosb)
		restart = 0
		if status == 0x80000006 {
			return names, nil
		}
		if status != 0 {
			return nil, ntErrno(status)
		}
		nameLen := *(*uint32)(unsafe.Pointer(&buf[60]))
		if nameLen%2 != 0 || 64+int(nameLen) > len(buf) {
			return nil, errors.New("a directory entry did not fit")
		}
		chars := unsafe.Slice((*uint16)(unsafe.Pointer(&buf[64])), nameLen/2)
		name := syscall.UTF16ToString(chars)
		if name != "." && name != ".." {
			names = append(names, name)
		}
	}
}

func markDeleted(h syscall.Handle) error {
	buf := [1]byte{1}
	var iosb ioStatusBlock
	status, _, _ := ntSetInformationFile.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&iosb)),
		uintptr(unsafe.Pointer(&buf[0])),
		1,
		13,
	)
	runtime.KeepAlive(buf)
	runtime.KeepAlive(&iosb)
	if status != 0 {
		return ntErrno(status)
	}
	return nil
}
