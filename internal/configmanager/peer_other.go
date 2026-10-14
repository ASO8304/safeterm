//go:build !linux

package configmanager

import (
	"fmt"
	"net"
)

func peerUID(*net.UnixConn) (uint32, error) {
	return 0, fmt.Errorf("configmanager: peer credentials are unavailable")
}
func peerPID(*net.UnixConn) (int32, error) { return 0, fmt.Errorf("peer credentials unavailable") }
