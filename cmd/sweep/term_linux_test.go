//go:build linux

package main_test

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

func openPTY() (*os.File, *os.File, error) {
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	if err := unix.IoctlSetPointerInt(int(pty.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		_ = pty.Close()
		return nil, nil, err
	}
	n, err := unix.IoctlGetInt(int(pty.Fd()), unix.TIOCGPTN)
	if err != nil {
		_ = pty.Close()
		return nil, nil, err
	}
	tty, err := os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		_ = pty.Close()
		return nil, nil, err
	}
	return pty, tty, nil
}
