//go:build !darwin

package sandbox

import (
	"context"
	"fmt"
	"time"
)

// ExecutableIdentity is unavailable when no host sandbox backend exists.
type ExecutableIdentity struct {
	Path   string
	SHA256 string
}

func ObserveProcessExecutable(processID int) (ExecutableIdentity, error) {
	return ExecutableIdentity{}, fmt.Errorf("process executable observation is unavailable on this operating system")
}

func VerifyProcessExecutable(processID int, expectedPath, expectedSHA256 string) (ExecutableIdentity, error) {
	return ExecutableIdentity{}, fmt.Errorf("process executable verification is unavailable on this operating system")
}

func VerifyProcessExecutableEventually(ctx context.Context, processID int, expectedPath, expectedSHA256 string, timeout time.Duration) (ExecutableIdentity, error) {
	return ExecutableIdentity{}, fmt.Errorf("process executable verification is unavailable on this operating system")
}
