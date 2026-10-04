//go:build unix

package agentprobe

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"ingen/sorna/execution"
)

const captureLimit = 1 << 20

type systemRunner struct{}

type capture struct {
	data      []byte
	truncated bool
}

func (c *capture) append(value []byte) {
	remaining := captureLimit - len(c.data)
	if remaining <= 0 {
		c.truncated = c.truncated || len(value) > 0
		return
	}
	if len(value) > remaining {
		c.data = append(c.data, value[:remaining]...)
		c.truncated = true
		return
	}
	c.data = append(c.data, value...)
}

func (systemRunner) Run(ctx context.Context, prepared execution.Prepared, workingDirectory string, env []string, stdin []byte) processResult {
	if err := ctx.Err(); err != nil {
		return processResult{Canceled: true, TimedOut: errors.Is(err, context.DeadlineExceeded)}
	}
	if len(prepared.Command) == 0 {
		return processResult{StartErr: true}
	}
	command := exec.Command(prepared.Command[0], prepared.Command[1:]...)
	command.Dir = workingDirectory
	command.Env = append([]string(nil), env...)
	command.Stdin = bytesReader(stdin)
	stdout := &capture{}
	stderr := &capture{}
	waitErr, startErr, canceled, timedOut, captureIncomplete := runCommandGroup(ctx, command, stdout, stderr)
	if startErr != nil {
		return processResult{StartErr: true, Canceled: canceled, TimedOut: timedOut}
	}
	exitCode := 0
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	} else if waitErr != nil {
		return processResult{Started: true, Canceled: canceled, TimedOut: timedOut, CaptureIncomplete: captureIncomplete}
	}
	var output []byte
	truncated := false
	if !captureIncomplete {
		output = make([]byte, 0, len(stdout.data)+len(stderr.data))
		output = append(output, stdout.data...)
		output = append(output, stderr.data...)
		truncated = stdout.truncated || stderr.truncated
	}
	return processResult{Started: true, ExitCode: &exitCode, Canceled: canceled, TimedOut: timedOut, Output: output, Truncated: truncated, CaptureIncomplete: captureIncomplete}
}

func runCommandGroup(ctx context.Context, command *exec.Cmd, stdout, stderr *capture) (waitErr, startErr error, canceled, timedOut, captureIncomplete bool) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		return nil, err, false, false, false
	}
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		return nil, err, false, false, false
	}
	command.Stdout, command.Stderr = stdoutWrite, stderrWrite
	if err := ctx.Err(); err != nil {
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		_ = stderrRead.Close()
		_ = stderrWrite.Close()
		return nil, err, true, errors.Is(err, context.DeadlineExceeded), false
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := command.Start(); err != nil {
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		_ = stderrRead.Close()
		_ = stderrWrite.Close()
		return nil, err, false, false, false
	}
	_ = stdoutWrite.Close()
	_ = stderrWrite.Close()
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go drainCapture(stdoutRead, stdout, stdoutDone)
	go drainCapture(stderrRead, stderr, stderrDone)
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	var timer *time.Timer
	var killAfter <-chan time.Time
	selectCancellation := ctx.Done()
	for {
		select {
		case waitErr = <-waitDone:
			// Reap only processes in the exact group created for this invocation.
			terminateDiagnosticGroup(command.Process.Pid)
			goto finished
		case <-selectCancellation:
			if !canceled {
				canceled = true
				timedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
				_ = syscall.Kill(-command.Process.Pid, syscall.SIGINT)
				timer = time.NewTimer(2 * time.Second)
				killAfter = timer.C
			}
			selectCancellation = nil
		case <-signals:
			if !canceled {
				canceled = true
				_ = syscall.Kill(-command.Process.Pid, syscall.SIGINT)
				timer = time.NewTimer(2 * time.Second)
				killAfter = timer.C
			} else {
				_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			}
		case <-killAfter:
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			killAfter = nil
		}
	}

finished:
	if timer != nil {
		timer.Stop()
	}
	if waitErr != nil && command.ProcessState == nil {
		terminateDiagnosticGroup(command.Process.Pid)
		captureIncomplete = true
	}
	if !joinDiagnosticDrain(stdoutDone, stdoutRead, command.Process.Pid) {
		captureIncomplete = true
	}
	if !joinDiagnosticDrain(stderrDone, stderrRead, command.Process.Pid) {
		captureIncomplete = true
	}
	return waitErr, nil, canceled, timedOut, captureIncomplete
}

func joinDiagnosticDrain(done <-chan struct{}, reader *os.File, pid int) bool {
	select {
	case <-done:
		return true
	case <-time.After(time.Second):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = reader.Close()
	}
	select {
	case <-done:
		return true
	case <-time.After(time.Second):
		return false
	}
}

func drainCapture(reader *os.File, output *capture, done chan<- struct{}) {
	defer close(done)
	defer reader.Close()
	chunk := make([]byte, 32<<10)
	for {
		count, err := reader.Read(chunk)
		if count > 0 {
			output.append(chunk[:count])
		}
		if err != nil {
			return
		}
	}
}

func terminateDiagnosticGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(100 * time.Millisecond)
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
