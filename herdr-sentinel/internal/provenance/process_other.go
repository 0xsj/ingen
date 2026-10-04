//go:build !unix

package provenance

import (
	"context"
	"errors"
	"os/exec"
)

func runCommand(ctx context.Context, command *exec.Cmd) (bool, error, bool) {
	if err := ctx.Err(); err != nil {
		return false, err, true
	}
	if err := command.Start(); err != nil {
		return false, err, false
	}
	done := make(chan struct{})
	monitorDone := make(chan struct{})
	canceled := make(chan struct{}, 1)
	go func() {
		defer close(monitorDone)
		select {
		case <-ctx.Done():
			select {
			case <-done:
				return
			default:
			}
			_ = command.Process.Kill()
			canceled <- struct{}{}
		case <-done:
		}
	}()
	err := command.Wait()
	close(done)
	<-monitorDone
	select {
	case <-canceled:
		return true, err, true
	default:
	}
	return true, err, false
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
