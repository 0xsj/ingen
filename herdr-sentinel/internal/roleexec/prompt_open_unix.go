//go:build unix

package roleexec

import (
	"os"
	"syscall"
)

func openPromptFile(root *os.Root, path string) (*os.File, error) {
	return root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
