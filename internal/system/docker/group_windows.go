//go:build windows

package docker

import "os/exec"

// group is a no-op on Windows. The deadline still closes the output pipes
// after dfPipeWait, so a child left holding them cannot hold the Scan open.
func group(cmd *exec.Cmd) {}
