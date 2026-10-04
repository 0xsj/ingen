//go:build !unix

package agentprobe

import "os"

func openExecutable(path string) (*os.File, error) {
	return os.Open(path)
}
