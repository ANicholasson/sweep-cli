//go:build unix

package docker

import (
	"os"
	"os/exec"
	"syscall"
)

// group starts the command in its own process group and kills that group
// when the deadline passes. Killing only the program would leave a child
// that inherited its output pipes, and the Scan would wait on those pipes.
func group(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == nil {
			return nil
		}
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return cmd.Process.Kill()
	}
}
