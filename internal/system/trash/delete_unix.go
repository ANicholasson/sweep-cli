//go:build linux || darwin

package trash

import (
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
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
	parent, err := openBeneath(filepath.Dir(abs), root)
	if err != nil {
		return err
	}
	defer func() { _ = syscall.Close(parent) }()
	return removeName(parent, filepath.Base(abs))
}

func removeName(dirfd int, name string) error {
	if name == "" || name == "." || name == ".." {
		return &os.PathError{Op: "delete", Path: name, Err: syscall.EINVAL}
	}
	err := unix.Unlinkat(dirfd, name, 0)
	if err == nil {
		return nil
	}
	if err != syscall.EISDIR && err != syscall.EPERM && err != syscall.EACCES {
		return &os.PathError{Op: "delete", Path: name, Err: err}
	}
	child, oerr := openChild(dirfd, name)
	if oerr != nil {
		return &os.PathError{Op: "delete", Path: name, Err: oerr}
	}
	defer func() { _ = syscall.Close(child) }()
	names, rerr := namesOf(child)
	if rerr != nil {
		return rerr
	}
	for _, n := range names {
		if n == "." || n == ".." {
			continue
		}
		if e := removeName(child, n); e != nil {
			return e
		}
	}
	if err := unix.Unlinkat(dirfd, name, unix.AT_REMOVEDIR); err != nil {
		return &os.PathError{Op: "delete", Path: name, Err: err}
	}
	return nil
}

func namesOf(fd int) ([]string, error) {
	dup, err := syscall.Dup(fd)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(dup), "")
	names, err := f.Readdirnames(-1)
	cerr := f.Close()
	if err != nil {
		return nil, err
	}
	return names, cerr
}
