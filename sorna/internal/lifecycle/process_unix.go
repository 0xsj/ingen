//go:build !windows

package lifecycle

import (
	"os/exec"
	"syscall"
)

func prepareCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalCommand(command *exec.Cmd, force bool) error {
	if command.Process == nil {
		return nil
	}
	signal := syscall.SIGINT
	if force {
		signal = syscall.SIGKILL
	}
	return syscall.Kill(-command.Process.Pid, signal)
}
