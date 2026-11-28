//go:build linux

package trash

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// openChild opens name as a directory from dirfd. A mount is refused, including
// a bind mount of the same filesystem: that mount has the same device number
// as its parent, and entering it would delete a path outside the Root.
// openat2 refuses the mount. Kernels before 5.6 return ENOSYS, and a seccomp
// profile can return EPERM. Those calls compare mount ids instead, and the
// error from that open is returned as itself.
func openChild(dirfd int, name string) (int, error) {
	how := &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_NO_XDEV,
	}
	fd, err := unix.Openat2(dirfd, name, how)
	if err == nil || !openat2Unavailable(err) {
		return fd, err
	}
	return openChildByMountID(dirfd, name)
}

func openat2Unavailable(err error) bool {
	return errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EPERM)
}

// openChildByMountID opens name when openat2 cannot. The mount id distinguishes
// a bind mount whose device number matches its parent.
func openChildByMountID(dirfd int, name string) (int, error) {
	child, err := openAt(dirfd, name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if err := sameMount(dirfd, child); err != nil {
		_ = syscall.Close(child)
		return -1, err
	}
	return child, nil
}

func sameMount(parent, child int) error {
	pid, err := mountID(parent)
	if err != nil {
		return err
	}
	cid, err := mountID(child)
	if err != nil {
		return err
	}
	if pid != cid {
		return syscall.EXDEV
	}
	return nil
}

func mountID(fd int) (int64, error) {
	b, err := os.ReadFile("/proc/self/fdinfo/" + strconv.Itoa(fd))
	if err != nil {
		return 0, err
	}
	for _, line := range bytes.Split(b, []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("mnt_id:")) {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) != 2 {
			return 0, syscall.EINVAL
		}
		return strconv.ParseInt(string(fields[1]), 10, 64)
	}
	return 0, syscall.ENOENT
}
