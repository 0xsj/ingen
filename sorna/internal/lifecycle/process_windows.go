//go:build windows

package lifecycle

import (
	"os"
	"os/exec"
)

func prepareCommand(command *exec.Cmd) {}

func signalCommand(command *exec.Cmd, force bool) error {
	if command.Process == nil {
		return nil
	}
	if force {
		return command.Process.Kill()
	}
	return command.Process.Signal(os.Interrupt)
}
