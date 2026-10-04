//go:build unix

package provenance

import (
	"context"
	"errors"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

const interruptGrace = 700 * time.Millisecond

func runCommand(ctx context.Context, command *exec.Cmd) (bool, error, bool) {
	if err := ctx.Err(); err != nil {
		return false, err, true
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return false, err, false
	}
	pid := command.Process.Pid
	done := make(chan struct{})
	monitorDone := make(chan struct{})
	var canceled atomic.Bool
	go func() {
		defer close(monitorDone)
		select {
		case <-ctx.Done():
			select {
			case <-done:
				return
			default:
			}
			canceled.Store(true)
			_ = syscall.Kill(-pid, syscall.SIGINT)
			timer := time.NewTimer(interruptGrace)
			defer timer.Stop()
			select {
			case <-done:
			case <-timer.C:
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		case <-done:
		}
	}()
	err := command.Wait()
	close(done)
	<-monitorDone
	// A command may leave descendants in its process group after exiting.
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	timer := time.NewTimer(100 * time.Millisecond)
	<-timer.C
	timer.Stop()
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		canceled.Store(true)
	}
	return true, err, canceled.Load()
}

func exitCode(command *exec.Cmd, err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	if command.ProcessState != nil {
		return command.ProcessState.ExitCode()
	}
	return 1
}
