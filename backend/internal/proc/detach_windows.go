//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
)

// Detach lets the child outlive this process (the installer stops and restarts the server that launched it).
// Not DETACHED_PROCESS: a console-less powershell.exe then exits at once without running its script.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | createNewProcessGroup}
}
