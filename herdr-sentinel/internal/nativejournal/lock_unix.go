//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package nativejournal

import (
	"fmt"
	"os"

	"syscall"
)

func lockPath(root *os.Root, relative string) (*os.File, error) {
	if err := checkPath(root, relative, true); err != nil {
		return nil, fmt.Errorf("native journal: lock path: %w", err)
	}
	file, err := root.OpenFile(relative, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("native journal: open lock: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("native journal: inspect lock: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("native journal: lock is not a regular file")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("native journal: acquire advisory lock: %w", err)
	}
	return file, nil
}

func unlockPath(file *os.File) {
	if file == nil {
		return
	}
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	_ = file.Close()
}
