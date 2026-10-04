//go:build !unix

package provenance

import "os"

func openExecutable(path string) (*os.File, error) {
	return os.Open(path)
}
