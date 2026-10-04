//go:build !unix

package agentprobe

import (
	"context"
	"errors"
	"os/exec"

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
	command := exec.Command(prepared.Command[0], prepared.Command[1:]...)
	command.Dir = workingDirectory
	command.Env = append([]string(nil), env...)
	command.Stdin = bytesReader(stdin)
	stdout, stderr := &capture{}, &capture{}
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		return processResult{StartErr: true}
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case waitErr := <-done:
		if waitErr != nil && command.ProcessState == nil {
			return processResult{Started: true}
		}
		code := command.ProcessState.ExitCode()
		output := append(append([]byte(nil), stdout.data...), stderr.data...)
		return processResult{Started: true, ExitCode: &code, Output: output, Truncated: stdout.truncated || stderr.truncated}
	case <-ctx.Done():
		_ = command.Process.Kill()
		<-done
		return processResult{Started: true, Canceled: true, TimedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}
	}
}
