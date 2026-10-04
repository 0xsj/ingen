//go:build !unix

package roleexec

import (
	"context"
	"os/exec"
)

func runWithCancellation(ctx context.Context, command *exec.Cmd) (error, error, bool, string) {
	if err := ctx.Err(); err != nil {
		return nil, err, true, "execution context canceled before child start"
	}
	if err := command.Start(); err != nil {
		return nil, err, false, ""
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		return err, nil, false, ""
	case <-ctx.Done():
		_ = command.Process.Kill()
		return <-done, nil, true, "execution context canceled"
	}
}
