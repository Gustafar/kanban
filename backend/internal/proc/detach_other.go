//go:build !windows

package proc

import "os/exec"

func Detach(cmd *exec.Cmd) {}
