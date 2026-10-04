//go:build !unix

package roleexec

import "os"

func openPromptFile(root *os.Root, path string) (*os.File, error) {
	return root.Open(path)
}
