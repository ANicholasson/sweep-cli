//go:build linux

package trash

import "golang.org/x/sys/unix"

// openChild opens name as a directory from dirfd. A mount is refused, including
// a bind mount of the same filesystem: that mount has the same device number
// as its parent, and entering it would delete a path outside the Root.
func openChild(dirfd int, name string) (int, error) {
	how := &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_NO_XDEV,
	}
	return unix.Openat2(dirfd, name, how)
}
