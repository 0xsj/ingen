//go:build unix

package roleexec

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func runWithCancellation(ctx context.Context, command *exec.Cmd) (error, error, bool, string) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := command.Start(); err != nil {
		return nil, err, false, ""
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	interrupted := false
	interruptReason := ""
	var killAfter <-chan time.Time
	cancelled := ctx.Done()
	for {
		select {
		case err := <-done:
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
			time.Sleep(100 * time.Millisecond)
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			return err, nil, interrupted, interruptReason
		case sig := <-signals:
			if !interrupted {
				var killErr error
				if s, ok := sig.(syscall.Signal); ok {
					killErr = syscall.Kill(-command.Process.Pid, s)
				} else {
					killErr = syscall.Kill(-command.Process.Pid, syscall.SIGINT)
				}
				if killErr == nil {
					interrupted = true
					interruptReason = sig.String()
					killAfter = time.After(2 * time.Second)
				}
			}
		case <-cancelled:
			if !interrupted {
				if syscall.Kill(-command.Process.Pid, syscall.SIGINT) == nil {
					interrupted = true
					interruptReason = "execution context canceled"
					killAfter = time.After(2 * time.Second)
				}
			}
			cancelled = nil
		case <-killAfter:
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			killAfter = nil
		}
	}
}
