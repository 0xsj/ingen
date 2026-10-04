//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package nativesession

import (
	"fmt"
	"os"
	"syscall"
)

func openAssessmentFile(root *os.Root, relative string) (*os.File, error) {
	return root.OpenFile(relative, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

func inspectLeaseLock(root, relative string) (string, error) {
	file, err := openRegularAssessmentFile(root, relative)
	if err != nil {
		return "", err
	}
	defer file.Close()
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		return "free", nil
	}
	if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
		return "held", nil
	}
	return "", fmt.Errorf("inspect advisory execution lease: %w", err)
}
