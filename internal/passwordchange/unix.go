//go:build linux

package passwordchange

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const maxFrame = 8192

type reply struct {
	Changed bool   `json:"changed"`
	Error   string `json:"error,omitempty"`
}

func peer(conn *net.UnixConn) (Peer, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return Peer{}, err
	}
	var credential *unix.Ucred
	var inner error
	err = raw.Control(func(fd uintptr) { credential, inner = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) })
	if err != nil {
		return Peer{}, err
	}
	if inner != nil {
		return Peer{}, inner
	}
	if credential == nil {
		return Peer{}, ErrDenied
	}
	return Peer{credential.Uid, credential.Pid}, nil
}
func Serve(ctx context.Context, path string, s *Service, allowed []uint32) error {
	permissions := map[uint32]bool{}
	for _, uid := range allowed {
		if uid == 0 {
			return ErrDenied
		}
		permissions[uid] = true
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return err
	}
	defer l.Close()
	if err := os.Chmod(path, 0660); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { l.Close() })
	defer stop()
	slots := make(chan struct{}, 16)
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
			go func() { defer func() { <-slots }(); handle(ctx, conn, s, permissions) }()
		default:
			conn.Close()
		}
	}
}
func handle(parent context.Context, conn *net.UnixConn, s *Service, allowed map[uint32]bool) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	p, err := peer(conn)
	if err != nil || !allowed[p.UID] {
		return
	}
	data, err := bufio.NewReader(io.LimitReader(conn, maxFrame+1)).ReadBytes('\n')
	if err != nil || len(data) > maxFrame {
		return
	}
	defer clear(data)
	var r Request
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil {
		return
	}
	defer clear(r.Current)
	defer clear(r.New)
	var extra any
	if d.Decode(&extra) != io.EOF {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	err = s.Change(ctx, p, r)
	result := reply{Changed: err == nil || err == ErrChanged}
	if err != nil {
		result.Error = err.Error()
	}
	_ = json.NewEncoder(conn).Encode(result)
}

type Client struct {
	Path string
	UID  uint32
}

func (c Client) Change(ctx context.Context, r Request) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.Path)
	if err != nil {
		return fmt.Errorf("password service unavailable; contact a Linux administrator")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	p, err := peer(conn.(*net.UnixConn))
	if err != nil || p.UID != c.UID {
		return ErrDenied
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	r.Version = 1
	data, err := json.Marshal(r)
	if err != nil {
		return ErrDenied
	}
	data = append(data, '\n')
	defer clear(data)
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("password change response unavailable; verify your password before retrying")
	}
	data, err = bufio.NewReader(io.LimitReader(conn, maxFrame+1)).ReadBytes('\n')
	if err != nil || len(data) > maxFrame {
		return fmt.Errorf("password change response unavailable; verify your password before retrying")
	}
	var result reply
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&result) != nil {
		return ErrDenied
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrDenied
	}
	if result.Changed && result.Error != "" {
		return ErrChanged
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	if !result.Changed {
		return ErrDenied
	}
	return nil
}
