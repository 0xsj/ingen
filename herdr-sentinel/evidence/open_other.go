//go:build !unix

package evidence

import "os"

func openEvidenceFile(root *os.Root, path string) (*os.File, error) {
	return root.Open(path)
}
