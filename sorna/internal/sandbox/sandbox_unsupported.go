//go:build !darwin

package sandbox

import "fmt"

func preparePlatform([]string, string, string, []string, []string, []string, []NetworkRule, []string) (Prepared, error) {
	return Prepared{}, fmt.Errorf("no host enforcement backend is available on this operating system")
}
