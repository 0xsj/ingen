//go:build unix

package roleexec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunWithCancellationReportsGracefulZeroExitAsCanceled(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", `trap 'exit 0' TERM; while :; do sleep 0.02; done`)
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}()
	waitErr, startErr, canceled, reason := runWithCancellation(context.Background(), command)
	if startErr != nil {
		t.Fatal(startErr)
	}
	if !canceled || reason != "terminated" {
		t.Fatalf("signal outcome canceled=%v reason=%q", canceled, reason)
	}
	if waitErr != nil {
		t.Fatalf("child handled termination with exit zero, got %v", waitErr)
	}
}

func TestRunWithCancellationKillsSignalIgnoringGroupAfterGrace(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "wrapper.pid")
	command := exec.Command("/bin/bash", "-c", `trap '' INT TERM; echo $$ > "$1"; while :; do :; done`, "bash", pidFile)
	cleanup := time.AfterFunc(8*time.Second, func() {
		if command.Process != nil {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		}
	})
	defer cleanup.Stop()
	signalSent := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(pidFile); err == nil {
				signalSent <- syscall.Kill(os.Getpid(), syscall.SIGTERM)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		signalSent <- fmt.Errorf("child did not publish readiness file before timeout")
	}()
	started := time.Now()
	waitErr, startErr, canceled, reason := runWithCancellation(context.Background(), command)
	elapsed := time.Since(started)
	if err := <-signalSent; err != nil {
		t.Fatal(err)
	}
	if startErr != nil {
		t.Fatal(startErr)
	}
	if !canceled || reason != "terminated" {
		t.Fatalf("signal outcome canceled=%v reason=%q", canceled, reason)
	}
	if elapsed < 1500*time.Millisecond || elapsed > 7*time.Second {
		t.Fatalf("ignored signal did not follow bounded grace period: elapsed=%s", elapsed)
	}
	exitErr, ok := waitErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("SIGKILL wait result = %v, want *exec.ExitError", waitErr)
	}
	waitStatus, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !waitStatus.Signaled() || waitStatus.Signal() != syscall.SIGKILL {
		t.Fatalf("signal-ignoring child wait status = %#v, want SIGKILL", exitErr.Sys())
	}
}

func TestRunWithCancellationTerminatesOnlyItsOwnedProcessGroup(t *testing.T) {
	dir := t.TempDir()
	unrelatedMarker := filepath.Join(dir, "unrelated.ticks")
	unrelated := exec.Command("/bin/bash", "-c", `while :; do echo tick >> "$1"; sleep 0.02; done`, "bash", unrelatedMarker)
	unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = unrelated.Process.Kill()
		_ = unrelated.Wait()
	}()
	descendantMarker := filepath.Join(dir, "descendant.ticks")
	pidFile := filepath.Join(dir, "descendant.pid")
	command := exec.Command("/bin/bash", "-c", `while :; do echo tick >> "$1"; sleep 0.02; done & echo $! > "$2"; exit 0`, "bash", descendantMarker, pidFile)
	waitErr, startErr, canceled, _ := runWithCancellation(context.Background(), command)
	if startErr != nil {
		t.Fatal(startErr)
	}
	if canceled || waitErr != nil {
		t.Fatalf("leader should exit normally, canceled=%v err=%v", canceled, waitErr)
	}
	contents, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read descendant PID: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid descendant PID %q: %v", contents, err)
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	if err := waitForFileContent(descendantMarker); err != nil {
		t.Fatalf("descendant did not start: %v", err)
	}
	if err := waitForFileContent(unrelatedMarker); err != nil {
		t.Fatalf("unrelated process did not start: %v", err)
	}
	deadCount, err := markerLineCount(descendantMarker)
	if err != nil {
		t.Fatal(err)
	}
	unrelatedBefore, err := markerLineCount(unrelatedMarker)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	deadAfter, err := markerLineCount(descendantMarker)
	if err != nil {
		t.Fatal(err)
	}
	unrelatedAfter, err := markerLineCount(unrelatedMarker)
	if err != nil {
		t.Fatal(err)
	}
	if deadAfter != deadCount {
		t.Fatalf("owned descendant PID %d continued running after leader exit (%d -> %d ticks)", pid, deadCount, deadAfter)
	}
	if unrelatedAfter <= unrelatedBefore {
		t.Fatalf("unrelated process group was affected (%d -> %d ticks)", unrelatedBefore, unrelatedAfter)
	}
}

func waitForFileContent(path string) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if contents, err := os.ReadFile(path); err == nil && len(contents) > 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("no content appeared in %s before timeout", path)
}

func markerLineCount(path string) (int, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strings.Count(string(contents), "tick\n"), nil
}
