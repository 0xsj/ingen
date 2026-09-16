//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package run

import (
	"fmt"
	"os"
)

func lockReceipt(path string) (*os.File, error) {
	return nil, fmt.Errorf("advisory receipt locking is unsupported on this platform")
}

func unlockReceipt(file *os.File) error {
	return nil
}
