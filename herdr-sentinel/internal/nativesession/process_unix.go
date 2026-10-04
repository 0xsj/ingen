//go:build unix

package nativesession

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const processInterruptGrace = 2 * time.Second
const containedRoleExecutionInterruptGrace = 5 * time.Second

// prepareCommandRunner registers OS signals before the wrapper claims child
// start, so a signal racing the authorization is buffered until the process
// group exists and can be interrupted safely.
func prepareCommandRunner(interruptGrace time.Duration) (func(*exec.Cmd, func() bool) (error, bool, bool, string, error), func()) {
	if interruptGrace <= 0 {
		interruptGrace = processInterruptGrace
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	stop := func() { signal.Stop(signals) }
	run := func(command *exec.Cmd, cancellationRequested func() bool) (error, bool, bool, string, error) {
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if cancellationRequested != nil && cancellationRequested() {
			return nil, true, false, "durable cancellation request before child start", nil
		}
		select {
		case sig := <-signals:
			return nil, true, false, sig.String(), nil
		default:
		}
		if err := command.Start(); err != nil {
			return err, false, false, "", nil
		}
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		interrupted := false
		interruptReason := ""
		var escalation *time.Timer
		var escalationC <-chan time.Time
		beginInterrupt := func(signal syscall.Signal, reason string) {
			if interrupted {
				return
			}
			err := syscall.Kill(-command.Process.Pid, signal)
			if err != nil && !errors.Is(err, syscall.ESRCH) {
				return
			}
			interrupted = true
			interruptReason = reason
			escalation = time.NewTimer(interruptGrace)
			escalationC = escalation.C
		}
		defer func() {
			if escalation != nil {
				escalation.Stop()
			}
		}()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case err := <-done:
				// Close descendants before the caller hashes output. A successful
				// direct-child Wait alone does not show that its process group has
				// stopped writing shared capture files.
				cleanupErr := cleanupProcessGroup(command.Process.Pid, interrupted, interruptGrace)
				if cleanupErr != nil {
					interruptReason = strings.TrimSpace(interruptReason + "; process-group cleanup timed out")
				}
				return err, interrupted, true, interruptReason, cleanupErr
			case sig := <-signals:
				if !interrupted {
					var forwarded syscall.Signal
					if s, ok := sig.(syscall.Signal); ok {
						forwarded = s
					} else {
						forwarded = syscall.SIGINT
					}
					beginInterrupt(forwarded, sig.String())
				}
			case <-escalationC:
				_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
				interruptReason += "; grace period expired; SIGKILL sent to child process group"
				escalationC = nil
			case <-ticker.C:
				if !interrupted && cancellationRequested != nil && cancellationRequested() {
					beginInterrupt(syscall.SIGINT, "durable cancellation request (SIGINT sent)")
				}
			}
		}
	}
	return run, stop
}

func waitProcessGroupGone(processID int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		err := syscall.Kill(-processID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.Is(syscall.Kill(-processID, 0), syscall.ESRCH)
}

func cleanupProcessGroup(processID int, force bool, grace time.Duration) error {
	if force {
		_ = syscall.Kill(-processID, syscall.SIGKILL)
		if waitProcessGroupGone(processID, grace) {
			return nil
		}
		return errors.New("owned process group did not exit after SIGKILL")
	}
	err := syscall.Kill(-processID, syscall.SIGTERM)
	if errors.Is(err, syscall.ESRCH) || waitProcessGroupGone(processID, grace) {
		return nil
	}
	_ = syscall.Kill(-processID, syscall.SIGKILL)
	if waitProcessGroupGone(processID, grace) {
		return nil
	}
	return errors.New("owned process group did not exit after SIGKILL")
}
