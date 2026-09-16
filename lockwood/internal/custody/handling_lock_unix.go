//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package custody

import (
	"fmt"
	"os"
	"syscall"
)

func withExclusiveHandlingLock(path string, fn func() error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open handling-event lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return fmt.Errorf("lock handling-event store: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}()
	return fn()
}
