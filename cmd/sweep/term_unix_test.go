//go:build linux || darwin

package main_test

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func startTerminal(t *testing.T, bin, dir string, env []string) *termSession {
	t.Helper()
	pty, tty, err := openPTY()
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.IoctlSetWinsize(int(pty.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 80, Col: 200}); err != nil {
		_ = pty.Close()
		_ = tty.Close()
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = errW
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		_ = pty.Close()
		_ = tty.Close()
		t.Fatal(err)
	}
	_ = tty.Close()
	_ = errW.Close()

	s := &termSession{in: pty, done: make(chan struct{})}
	go func() {
		_, _ = io.Copy(s, errR)
	}()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := pty.Read(buf)
			if n > 0 {
				s.appendOut(append([]byte(nil), buf[:n]...))
			}
			if readErr != nil {
				return
			}
		}
	}()
	go func() {
		waitErr := cmd.Wait()
		code := 0
		if waitErr != nil {
			var exit *exec.ExitError
			if errors.As(waitErr, &exit) {
				code = exit.ExitCode()
			} else {
				s.err = waitErr
				code = -1
			}
		}
		s.exit = code
		close(s.done)
	}()
	s.stop = func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return s
}
