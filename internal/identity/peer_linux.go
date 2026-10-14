//go:build linux

package identity

import (
	"golang.org/x/sys/unix"
	"net"
)

func peer(c *net.UnixConn) (Peer, error) {
	raw, e := c.SyscallConn()
	if e != nil {
		return Peer{}, e
	}
	var p *unix.Ucred
	var socketErr error
	e = raw.Control(func(fd uintptr) { p, socketErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) })
	if e != nil {
		return Peer{}, e
	}
	if socketErr != nil {
		return Peer{}, socketErr
	}
	if p == nil {
		return Peer{}, ErrDenied
	}
	return Peer{p.Uid, p.Pid}, nil
}
