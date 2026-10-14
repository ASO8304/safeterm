//go:build !linux

package identity

import "net"

func peer(*net.UnixConn) (Peer, error) { return Peer{}, ErrDenied }
