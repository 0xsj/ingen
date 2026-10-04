//go:build darwin

package herdrclient

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"unsafe"
)

const (
	darwinSOLLocal       = 0
	darwinLocalPeerCred  = 1
	darwinXUCredVersion0 = 0
)

func socketOwnerUID(info os.FileInfo) (uint32, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, errors.New("socket metadata has no Darwin stat owner")
	}
	return stat.Uid, nil
}

func platformPeerUID(conn net.Conn) (uint32, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, errors.New("connected transport is not a UNIX socket")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var callErr error
	err = raw.Control(func(fd uintptr) {
		// Darwin struct xucred is 76 bytes in the SDK (version, uid, group
		// count, alignment, and NGROUPS gids). Only version and uid are used.
		credentials := make([]byte, 76)
		length := uint32(len(credentials))
		_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, darwinSOLLocal, darwinLocalPeerCred,
			uintptr(unsafe.Pointer(&credentials[0])), uintptr(unsafe.Pointer(&length)), 0)
		if errno != 0 {
			callErr = errno
			return
		}
		if length < 8 {
			callErr = fmt.Errorf("LOCAL_PEERCRED returned only %d bytes", length)
			return
		}
		if binary.NativeEndian.Uint32(credentials[0:4]) != darwinXUCredVersion0 {
			callErr = fmt.Errorf("LOCAL_PEERCRED returned unsupported xucred version %d", binary.NativeEndian.Uint32(credentials[0:4]))
			return
		}
		uid = binary.NativeEndian.Uint32(credentials[4:8])
	})
	if err != nil {
		return 0, err
	}
	if callErr != nil {
		return 0, callErr
	}
	return uid, nil
}
