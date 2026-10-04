//go:build unix

package agentprobe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"ingen/sorna/execution"
)

func TestSystemRunnerCancellationTerminatesDescendantHoldingCapturePipe(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	pidFile := filepath.Join(work, "descendant.pid")
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan processResult, 1)
	prepared := execution.Prepared{Command: []string{binary, "-test.run=^TestAgentProbeProcessHelper$"}}
	go func() {
		resultCh <- (systemRunner{}).Run(ctx, prepared, work, []string{
			"PATH=/usr/bin:/bin", "AGENTPROBE_HELPER=1", "AGENTPROBE_MODE=parent", "AGENTPROBE_PID_FILE=" + pidFile,
		}, nil)
	}()
	t.Cleanup(func() {
		cancel()
		if data, readErr := os.ReadFile(pidFile); readErr == nil {
			if pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil && pid > 1 {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not start and record its exact descendant PID")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case result := <-resultCh:
		if !result.Started || !result.Canceled || result.TimedOut || result.ExitCode == nil || *result.ExitCode != 0 {
			t.Fatalf("unexpected managed cancellation result: %#v", result)
		}
		if result.CaptureIncomplete || result.Truncated {
			t.Fatalf("descendant cleanup did not complete bounded output drain: %#v", result)
		}
		if !strings.Contains(string(result.Output), "agentprobe-parent-ready") {
			t.Fatalf("helper readiness marker missing from captured output: %q", result.Output)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("runner did not finish bounded cancellation and descendant cleanup")
	}
}

func TestAgentProbeProcessHelper(t *testing.T) {
	if os.Getenv("AGENTPROBE_HELPER") != "1" {
		return
	}
	switch os.Getenv("AGENTPROBE_MODE") {
	case "descendant":
		signal.Ignore(syscall.SIGINT, syscall.SIGTERM)
		for {
			time.Sleep(time.Second)
		}
	case "parent":
		pidFile := os.Getenv("AGENTPROBE_PID_FILE")
		if pidFile == "" {
			os.Exit(81)
		}
		signals := make(chan os.Signal, 2)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		child := exec.Command(os.Args[0], "-test.run=^TestAgentProbeProcessHelper$")
		child.Env = []string{"PATH=/usr/bin:/bin", "AGENTPROBE_HELPER=1", "AGENTPROBE_MODE=descendant"}
		if err := child.Start(); err != nil {
			os.Exit(82)
		}
		if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", child.Process.Pid)), 0600); err != nil {
			_ = child.Process.Kill()
			os.Exit(83)
		}
		fmt.Fprintln(os.Stdout, "agentprobe-parent-ready")
		<-signals
		os.Exit(0)
	default:
		os.Exit(84)
	}
}
