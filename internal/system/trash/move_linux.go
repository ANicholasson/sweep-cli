//go:build linux

package trash

import (
	"os"
	"path/filepath"
	"syscall"
)

// Open is the trash in $XDG_DATA_HOME/Trash, or in
// $HOME/.local/share/Trash when XDG_DATA_HOME is unset. The directory is
// created when a path is moved, not before.
func Open() (Mover, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Mover{}, err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return In(filepath.Join(dir, "Trash")), nil
}

func (m Mover) locate(string, string) (string, error) {
	if m.dir == "" {
		return "", ErrUnavailable
	}
	return m.dir, nil
}

func itemDirs(trash string) (string, string) {
	return filepath.Join(trash, "files"), filepath.Join(trash, "info")
}

func renameAt(olddirfd int, oldpath string, newdirfd int, newpath string) error {
	return syscall.Renameat(olddirfd, oldpath, newdirfd, newpath)
}

func openAt(dirfd int, path string, flags int, mode uint32) (int, error) {
	return syscall.Openat(dirfd, path, flags, mode)
}
