//go:build unix

package codexbroker_test

import (
	"os/exec"
	"syscall"
	"time"
)

func prepareInstalledProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
}

func cleanupInstalledProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		// Only the process group created for this explicit test invocation.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		time.Sleep(100 * time.Millisecond)
	}
}
