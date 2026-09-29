//go:build !linux

package audit

import (
	"fmt"
	"net"
)

type peerCredentials struct {
	PID      int32
	UID, GID uint32
}

func credentialsFor(_ *net.UnixConn) (peerCredentials, error) {
	return peerCredentials{}, fmt.Errorf("audit: authenticated Unix IPC is supported only on Linux")
}
