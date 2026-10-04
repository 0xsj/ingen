//go:build unix

package provenance

import (
	"os"
	"syscall"
)

func openProvenanceInput(root *os.Root, path string) (*os.File, error) {
	return root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
