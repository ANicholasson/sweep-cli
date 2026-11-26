//go:build darwin

package trash

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// Open is the user's Trash. A Candidate on the same volume as the home
// directory goes to $HOME/.Trash. A Candidate on another volume goes to
// that volume's .Trashes directory for this user. The directory is
// created when a path is moved, not before.
func Open() (Mover, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Mover{}, err
	}
	if home == "" {
		return Mover{}, ErrUnavailable
	}
	return Mover{home: home}, nil
}

func (m Mover) locate(abs, root string) (string, error) {
	if m.dir != "" {
		return m.dir, nil
	}
	if m.home == "" {
		return "", ErrUnavailable
	}
	return volumeTrash(m.home, abs, root)
}

func itemDirs(trash string) (string, string) {
	return trash, filepath.Join(trash, ".info")
}

func renameAt(olddirfd int, oldpath string, newdirfd int, newpath string) error {
	return unix.Renameat(olddirfd, oldpath, newdirfd, newpath)
}

func openAt(dirfd int, path string, flags int, mode uint32) (int, error) {
	return unix.Openat(dirfd, path, flags, mode)
}

// volumeTrash is $HOME/.Trash when path is on the home volume, and
// <volume>/.Trashes/<uid> otherwise. A rename stays on one file system,
// so the other volume has its own trash.
func volumeTrash(home, path, root string) (string, error) {
	homeDev, err := device(home)
	if err != nil {
		return "", err
	}
	pathDev, err := deviceOf(path)
	if err != nil {
		return "", err
	}
	vol := ""
	if pathDev != homeDev {
		resolved, err := resolvedForVolume(path, root)
		if err != nil {
			return "", err
		}
		vol, err = volumeRoot(resolved, pathDev)
		if err != nil {
			return "", err
		}
	}
	return trashDir(home, homeDev, pathDev, vol, os.Getuid()), nil
}

// resolvedForVolume resolves a Link above root. A Link at root or beneath
// it stays in the spelled path. The rename refuses that Link on its own.
func resolvedForVolume(path, root string) (string, error) {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if root != "" && contains(root, path) {
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		if rel == "." {
			return resolved, nil
		}
		return filepath.Join(resolved, rel), nil
	}
	return filepath.EvalSymlinks(path)
}

func trashDir(home string, homeDev, pathDev uint64, root string, uid int) string {
	if pathDev == homeDev {
		return filepath.Join(home, ".Trash")
	}
	return filepath.Join(root, ".Trashes", strconv.Itoa(uid))
}

func device(path string) (uint64, error) {
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Dev), nil
}

func deviceOf(path string) (uint64, error) {
	cur := filepath.Clean(path)
	for {
		dev, err := device(cur)
		if err == nil {
			return dev, nil
		}
		if !os.IsNotExist(err) {
			return 0, err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return 0, err
		}
		cur = parent
	}
}

func volumeRoot(path string, dev uint64) (string, error) {
	cur := filepath.Clean(path)
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return cur, nil
		}
		pdev, err := device(parent)
		if err != nil {
			if os.IsNotExist(err) {
				cur = parent
				continue
			}
			return "", err
		}
		if pdev != dev {
			return cur, nil
		}
		cur = parent
	}
}
