//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// Hide stops child console programs (git, claude, powershell) from flashing a window
// when the server itself runs without a console.
func Hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
