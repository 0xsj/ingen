//go:build !unix

package nativesession

import (
	"os/exec"
	"time"
)

// Platforms without process-group signal support can still run the wrapper;
// durable cancellation remains observed when the child exits.
func prepareCommandRunner(_ time.Duration) (func(*exec.Cmd, func() bool) (error, bool, bool, string, error), func()) {
	run := func(command *exec.Cmd, cancellationRequested func() bool) (error, bool, bool, string, error) {
		if cancellationRequested != nil && cancellationRequested() {
			return nil, true, false, "durable cancellation request before child start", nil
		}
		if err := command.Start(); err != nil {
			return err, false, false, "", nil
		}
		err := command.Wait()
		interrupted := cancellationRequested != nil && cancellationRequested()
		reason := ""
		if interrupted {
			reason = "durable cancellation request"
		}
		return err, interrupted, true, reason, nil
	}
	return run, func() {}
}
