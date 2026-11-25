//go:build unix

package files

import (
	"io/fs"
	"os"
	"syscall"
)

// openDir opens dir without following a Link at the end of the path. A
// directory swapped for a Link between the listing that named it and this
// open is not followed. A Link among the ancestors is followed, which is
// how a Root that was resolved once is listed.
func openDir(dir string) (*os.File, error) {
	fd, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, &fs.PathError{Op: "list", Path: dir, Err: err}
	}
	return os.NewFile(uintptr(fd), dir), nil
}
