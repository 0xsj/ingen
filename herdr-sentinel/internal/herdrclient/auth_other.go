//go:build !darwin && !linux

package herdrclient

import (
	"errors"
	"net"
	"os"
)

func socketOwnerUID(os.FileInfo) (uint32, error) {
	return 0, errors.New("UNIX socket owner inspection is unsupported on this platform")
}

func platformPeerUID(net.Conn) (uint32, error) {
	return 0, errors.New("UNIX peer credential inspection is unsupported on this platform")
}
