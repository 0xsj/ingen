//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ExecutableIdentity is the host-observed identity of a live process.
type ExecutableIdentity struct {
	Path   string
	SHA256 string
}

// ObserveProcessExecutable resolves the executable path for a live Darwin
// process and hashes the bytes at that path. It is an observation by the Sorna
// parent, not an independent OS attestation.
func ObserveProcessExecutable(processID int) (ExecutableIdentity, error) {
	if processID <= 0 {
		return ExecutableIdentity{}, fmt.Errorf("process ID must be positive")
	}
	path, err := processExecutablePath(processID)
	if err != nil {
		return ExecutableIdentity{}, err
	}
	return identityForPath(processID, path)
}

func processExecutablePath(processID int) (string, error) {
	output, err := exec.Command("/bin/ps", "-p", strconv.Itoa(processID), "-o", "comm=").Output()
	if err != nil {
		return "", fmt.Errorf("resolve executable for process %d: %w", processID, err)
	}
	path := canonicalizeExistingParent(strings.TrimSpace(string(output)))
	if path == "" {
		return "", fmt.Errorf("resolve executable for process %d: ps returned no path", processID)
	}
	return path, nil
}

func identityForPath(processID int, path string) (ExecutableIdentity, error) {
	digest, err := hashExecutable(path)
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("hash executable for process %d: %w", processID, err)
	}
	return ExecutableIdentity{Path: path, SHA256: digest}, nil
}

// VerifyProcessExecutable compares a host observation with the identity
// prepared before launch. When the root is a sandbox-exec wrapper, it also
// checks descendants for the prepared workload executable.
func VerifyProcessExecutable(processID int, expectedPath, expectedSHA256 string) (ExecutableIdentity, error) {
	observed, err := ObserveProcessExecutable(processID)
	if err != nil {
		return ExecutableIdentity{}, err
	}
	expectedPath = canonicalizeExistingParent(expectedPath)
	if observed.Path == expectedPath && observed.SHA256 == expectedSHA256 {
		return observed, nil
	}

	entries, tableErr := processTable()
	if tableErr == nil {
		if entry, ok := entries[processID]; ok && canonicalizeExistingParent(entry.Path) == expectedPath {
			current, identityErr := identityForPath(processID, expectedPath)
			if identityErr == nil {
				if current.SHA256 == expectedSHA256 {
					return current, nil
				}
				return current, executableIdentityMismatch(processID, current, expectedPath, expectedSHA256)
			}
		}
		for _, descendantID := range descendantProcessIDs(processID, entries) {
			path := entries[descendantID].Path
			if canonicalizeExistingParent(path) != expectedPath {
				continue
			}
			descendant, identityErr := identityForPath(descendantID, expectedPath)
			if identityErr != nil {
				continue
			}
			if descendant.SHA256 == expectedSHA256 {
				return descendant, nil
			}
			return descendant, executableIdentityMismatch(processID, descendant, expectedPath, expectedSHA256)
		}
	}
	if tableErr != nil {
		return observed, fmt.Errorf("%w; inspect descendants: %v", executableIdentityMismatch(processID, observed, expectedPath, expectedSHA256), tableErr)
	}
	return observed, executableIdentityMismatch(processID, observed, expectedPath, expectedSHA256)
}

// VerifyProcessExecutableEventually retries the host observation while a
// launcher is still settling into its workload process. This is useful for
// sandbox wrappers that fork or exec asynchronously immediately after start.
func VerifyProcessExecutableEventually(ctx context.Context, processID int, expectedPath, expectedSHA256 string, timeout time.Duration) (ExecutableIdentity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var lastObserved ExecutableIdentity
	var lastErr error
	for {
		lastObserved, lastErr = VerifyProcessExecutable(processID, expectedPath, expectedSHA256)
		if lastErr == nil {
			return lastObserved, nil
		}
		select {
		case <-ctx.Done():
			return lastObserved, ctx.Err()
		case <-deadline.C:
			return lastObserved, lastErr
		case <-ticker.C:
		}
	}
}

func executableIdentityMismatch(processID int, observed ExecutableIdentity, expectedPath, expectedSHA256 string) error {
	return fmt.Errorf("process %d executable identity mismatch: observed %s (%s), expected %s (%s)", processID, observed.Path, observed.SHA256, expectedPath, expectedSHA256)
}

type processEntry struct {
	PID  int
	PPID int
	Path string
}

func processTable() (map[int]processEntry, error) {
	output, err := exec.Command("/bin/ps", "-axo", "pid=,ppid=,comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("read process table: %w", err)
	}
	entries := make(map[int]processEntry)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		if pidErr != nil || ppidErr != nil {
			continue
		}
		path := strings.TrimSpace(strings.Join(fields[2:], " "))
		if path == "" {
			continue
		}
		entries[pid] = processEntry{PID: pid, PPID: ppid, Path: path}
	}
	return entries, nil
}

func descendantProcessIDs(rootID int, entries map[int]processEntry) []int {
	children := make(map[int][]int)
	for _, entry := range entries {
		children[entry.PPID] = append(children[entry.PPID], entry.PID)
	}
	ids := make([]int, 0)
	queue := []int{rootID}
	seen := map[int]bool{rootID: true}
	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]
		for _, childID := range children[parentID] {
			if seen[childID] {
				continue
			}
			seen[childID] = true
			ids = append(ids, childID)
			queue = append(queue, childID)
		}
	}
	return ids
}
