//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package nativesession

import (
	"fmt"
	"os"
)

func openAssessmentFile(root *os.Root, relative string) (*os.File, error) {
	return root.Open(relative)
}

func inspectLeaseLock(_ string, _ string) (string, error) {
	return "", fmt.Errorf("read-only advisory execution lease inspection is unsupported on this platform")
}
