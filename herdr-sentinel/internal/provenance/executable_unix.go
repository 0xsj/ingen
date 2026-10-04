//go:build unix

package provenance

import (
	"os"
	"syscall"
)

func openExecutable(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
