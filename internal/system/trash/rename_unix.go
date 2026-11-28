//go:build linux || darwin

package trash

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func renameBetween(oldpath, oldRoot string, followOld bool, newpath, newRoot string, followNew bool) error {
	oldfd, err := openParent(oldpath, oldRoot, followOld)
	if err != nil {
		return err
	}
	defer func() { _ = syscall.Close(oldfd) }()
	newfd, err := openParent(newpath, newRoot, followNew)
	if err != nil {
		return err
	}
	defer func() { _ = syscall.Close(newfd) }()
	if err := renameAt(oldfd, filepath.Base(oldpath), newfd, filepath.Base(newpath)); err != nil {
		return &os.PathError{Op: "move", Path: oldpath, Err: err}
	}
	return nil
}

func openParent(path, root string, follow bool) (int, error) {
	dir := filepath.Dir(path)
	if follow {
		return openDirFollow(dir)
	}
	return openBeneath(dir, root)
}

func openBeneath(dir, root string) (int, error) {
	dir = filepath.Clean(dir)
	root = filepath.Clean(root)
	if root == "" || !contains(root, dir) {
		return openDirNoFollow(dir)
	}
	fd, err := openFinalNoFollow(root)
	if err != nil {
		return -1, err
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		_ = syscall.Close(fd)
		return openDirNoFollow(dir)
	}
	if rel == "." {
		return fd, nil
	}
	for _, name := range strings.Split(rel, string(filepath.Separator)) {
		if name == "" || name == "." {
			continue
		}
		// openChild refuses a mount, including a bind mount of the same
		// filesystem. A mount between the Root and this directory is not
		// entered, so a path outside the Root is not deleted.
		next, err := openChild(fd, name)
		_ = syscall.Close(fd)
		if err != nil {
			return -1, &os.PathError{Op: "open", Path: dir, Err: err}
		}
		fd = next
	}
	return fd, nil
}

func contains(root, path string) bool {
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

func openFinalNoFollow(path string) (int, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return fd, nil
}

func openDirFollow(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return -1, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return -1, err
	}
	if !info.IsDir() {
		_ = f.Close()
		return -1, &os.PathError{Op: "open", Path: path, Err: errors.New("not a directory")}
	}
	fd, err := syscall.Dup(int(f.Fd()))
	cerr := f.Close()
	if err != nil {
		return -1, err
	}
	if cerr != nil {
		_ = syscall.Close(fd)
		return -1, cerr
	}
	return fd, nil
}

func openDirNoFollow(path string) (int, error) {
	path = filepath.Clean(path)
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if path == "/" {
		return fd, nil
	}
	if !strings.HasPrefix(path, "/") {
		_ = syscall.Close(fd)
		return -1, &os.PathError{Op: "open", Path: path, Err: errors.New("not absolute")}
	}
	for _, name := range strings.Split(path[1:], "/") {
		next, err := openAt(fd, name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
		_ = syscall.Close(fd)
		if err != nil {
			return -1, &os.PathError{Op: "open", Path: path, Err: err}
		}
		fd = next
	}
	return fd, nil
}
