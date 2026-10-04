//go:build unix

package evidence

import (
	"os"
	"syscall"
)

func openEvidenceFile(root *os.Root, path string) (*os.File, error) {
	return root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
