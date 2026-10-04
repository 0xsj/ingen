//go:build !unix

package provenance

import "os"

func openProvenanceInput(root *os.Root, path string) (*os.File, error) {
	return root.Open(path)
}
