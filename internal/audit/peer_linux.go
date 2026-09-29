//go:build linux

package audit

import (
	"fmt"
	"net"
	"syscall"
)

type peerCredentials struct {
	PID      int32
	UID, GID uint32
}

func credentialsFor(conn *net.UnixConn) (peerCredentials, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return peerCredentials{}, err
	}
	var credentials *syscall.Ucred
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, socketErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return peerCredentials{}, err
	}
	if socketErr != nil {
		return peerCredentials{}, socketErr
	}
	if credentials == nil {
		return peerCredentials{}, fmt.Errorf("audit: missing peer credentials")
	}
	return peerCredentials{PID: credentials.Pid, UID: credentials.Uid, GID: credentials.Gid}, nil
}
