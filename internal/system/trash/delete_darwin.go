//go:build darwin

package trash

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func openChild(dirfd int, name string) (int, error) {
	child, err := openAt(dirfd, name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if err := sameDevice(dirfd, child); err != nil {
		_ = syscall.Close(child)
		return -1, err
	}
	return child, nil
}

func sameDevice(parent, child int) error {
	var ps, cs unix.Stat_t
	if err := unix.Fstat(parent, &ps); err != nil {
		return err
	}
	if err := unix.Fstat(child, &cs); err != nil {
		return err
	}
	if ps.Dev != cs.Dev {
		return syscall.EXDEV
	}
	return nil
}
