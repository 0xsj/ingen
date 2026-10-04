//go:build unix

package nativesession

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// prepareCommandRunner registers OS signals before the wrapper claims child
// start, so a signal racing the authorization is buffered until the process
// group exists and can be interrupted safely.
func prepareCommandRunner() (func(*exec.Cmd, func() bool) (error, bool, bool, string), func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	stop := func() { signal.Stop(signals) }
	run := func(command *exec.Cmd, cancellationRequested func() bool) (error, bool, bool, string) {
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if cancellationRequested != nil && cancellationRequested() {
			return nil, true, false, "durable cancellation request before child start"
		}
		select {
		case sig := <-signals:
			return nil, true, false, sig.String()
		default:
		}
		if err := command.Start(); err != nil {
			return err, false, false, ""
		}
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		interrupted := false
		interruptReason := ""
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case err := <-done:
				return err, interrupted, true, interruptReason
			case sig := <-signals:
				if !interrupted {
					var killErr error
					if s, ok := sig.(syscall.Signal); ok {
						killErr = syscall.Kill(-command.Process.Pid, s)
					} else {
						killErr = syscall.Kill(-command.Process.Pid, syscall.SIGINT)
					}
					interrupted = killErr == nil
					if interrupted {
						interruptReason = sig.String()
					}
				}
			case <-ticker.C:
				if !interrupted && cancellationRequested != nil && cancellationRequested() {
					interrupted = syscall.Kill(-command.Process.Pid, syscall.SIGINT) == nil
					if interrupted {
						interruptReason = "durable cancellation request (SIGINT sent)"
					}
				}
			}
		}
	}
	return run, stop
}
