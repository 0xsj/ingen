//go:build linux

package herdrclient

import (
	"errors"
	"net"
	"os"
	"syscall"
)

func socketOwnerUID(info os.FileInfo) (uint32, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, errors.New("socket metadata has no Linux stat owner")
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
		credentials, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if err != nil {
			callErr = err
			return
		}
		uid = credentials.Uid
	})
	if err != nil {
		return 0, err
	}
	if callErr != nil {
		return 0, callErr
	}
	return uid, nil
}
