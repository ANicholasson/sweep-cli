//go:build darwin

package main_test

import (
	"bytes"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

func openPTY() (*os.File, *os.File, error) {
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	if err := unix.IoctlSetInt(int(pty.Fd()), unix.TIOCPTYGRANT, 0); err != nil {
		_ = pty.Close()
		return nil, nil, err
	}
	if err := unix.IoctlSetInt(int(pty.Fd()), unix.TIOCPTYUNLK, 0); err != nil {
		_ = pty.Close()
		return nil, nil, err
	}
	name, err := darwinPTYName(pty)
	if err != nil {
		_ = pty.Close()
		return nil, nil, err
	}
	tty, err := os.OpenFile(name, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		_ = pty.Close()
		return nil, nil, err
	}
	return pty, tty, nil
}

func darwinPTYName(pty *os.File) (string, error) {
	buf := make([]byte, 128)
	// TIOCPTYGNAME writes the name into buf. IoctlSetInt is the libSystem
	// ioctl, and on this architecture the pointer fits in the argument.
	err := unix.IoctlSetInt(int(pty.Fd()), unix.TIOCPTYGNAME, int(uintptr(unsafe.Pointer(&buf[0]))))
	runtime.KeepAlive(buf)
	if err != nil {
		return "", err
	}
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		buf = buf[:i]
	}
	return string(buf), nil
}
