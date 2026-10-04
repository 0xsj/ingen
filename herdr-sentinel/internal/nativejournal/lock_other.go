//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package nativejournal

import (
	"fmt"
	"os"
)

func lockPath(_ *os.Root, _ string) (*os.File, error) {
	return nil, fmt.Errorf("native journal: process-shared advisory locking is unsupported on this platform")
}

func unlockPath(file *os.File) {
	if file != nil {
		_ = file.Close()
	}
}
