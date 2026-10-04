package herdrclient

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func validateSocketEndpoint(path string) error {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("Herdr socket path must be an explicit canonical absolute path (resolve aliases such as /tmp before selecting the endpoint)")
	}
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	remainder := strings.TrimPrefix(path, current)
	endpointChecked := false
	for _, part := range strings.Split(remainder, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect socket path component %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("socket path component %q is a symbolic link", current)
		}
		if current == path {
			endpointChecked = true
			if info.Mode()&os.ModeSocket == 0 {
				return fmt.Errorf("endpoint %q is not a UNIX socket", path)
			}
			uid, err := socketOwnerUID(info)
			if err != nil {
				return fmt.Errorf("inspect socket owner: %w", err)
			}
			if uid != uint32(os.Geteuid()) {
				return fmt.Errorf("socket owner UID %d does not match caller effective UID %d", uid, os.Geteuid())
			}
		}
	}
	if !endpointChecked {
		return fmt.Errorf("Herdr socket path does not identify a socket endpoint")
	}
	return nil
}

func peerUID(conn net.Conn) (uint32, error) { return platformPeerUID(conn) }
