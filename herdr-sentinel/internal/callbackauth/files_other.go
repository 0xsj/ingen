//go:build !darwin && !linux

package callbackauth

import "fmt"

func readPrivateKeyFile(string) ([]byte, error) {
	return nil, fmt.Errorf("authenticated callback key files are unsupported on this operating system")
}

func readRegularFile(string, int64) ([]byte, error) {
	return nil, fmt.Errorf("authenticated callback file ingress is unsupported on this operating system")
}
