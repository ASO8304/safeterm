package identity

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"time"
)

const maxFrame = 8192

type request struct {
	Version  int    `json:"version"`
	Action   string `json:"action"`
	UserID   string `json:"user_id,omitempty"`
	Password []byte `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
	PeerUID  uint32 `json:"peer_uid,omitempty"`
	PeerPID  int32  `json:"peer_pid,omitempty"`
}
type response struct {
	Session *Session `json:"session,omitempty"`
	Error   string   `json:"error,omitempty"`
}
type Server struct {
	listener  *net.UnixListener
	service   *Service
	console   uint32
	verifiers map[uint32]bool
	slots     chan struct{}
}

func Listen(path string, service *Service, console uint32, verifiers []uint32) (*Server, error) {
	if service == nil || path == "" || len(verifiers) == 0 {
		return nil, errors.New("identity: invalid listener settings")
	}
	allowed := map[uint32]bool{}
	for _, uid := range verifiers {
		if uid == console || allowed[uid] {
			return nil, errors.New("identity: peer identities must be distinct")
		}
		allowed[uid] = true
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0660); err != nil {
		l.Close()
		return nil, err
	}
	return &Server{l, service, console, allowed, make(chan struct{}, 32)}, nil
}
func (s *Server) Close() error { return s.listener.Close() }
func (s *Server) Serve(ctx context.Context) error {
	stop := context.AfterFunc(ctx, func() { s.listener.Close() })
	defer stop()
	for {
		c, err := s.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case s.slots <- struct{}{}:
			go func() { defer func() { <-s.slots }(); s.handle(c) }()
		default:
			c.Close()
		}
	}
}
func (s *Server) handle(c *net.UnixConn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	p, err := peer(c)
	if err != nil || p.UID != s.console && !s.verifiers[p.UID] {
		return
	}
	data, err := bufio.NewReader(io.LimitReader(c, maxFrame+1)).ReadBytes('\n')
	if err != nil || len(data) > maxFrame {
		return
	}
	var req request
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || req.Version != 1 {
		return
	}
	defer clear(req.Password)
	defer clear(data)
	var extra any
	if d.Decode(&extra) != io.EOF {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	reply := response{Error: ErrDenied.Error()}
	switch req.Action {
	case "login":
		if p.UID != s.console || req.Token != "" || req.PeerUID != 0 || req.PeerPID != 0 {
			return
		}
		session, e := s.service.Login(ctx, p, req.UserID, req.Password)
		if e == nil {
			reply = response{Session: &session}
		}
	case "verify":
		if !s.verifiers[p.UID] || req.PeerUID != s.console || req.UserID != "" || len(req.Password) != 0 {
			return
		}
		session, e := s.service.Verify(req.Token, Peer{req.PeerUID, req.PeerPID})
		if e == nil {
			reply = response{Session: &session}
		}
	case "logout":
		if p.UID != s.console || req.UserID != "" || len(req.Password) != 0 || req.PeerUID != 0 || req.PeerPID != 0 {
			return
		}
		if s.service.Logout(ctx, req.Token, p) == nil {
			reply = response{}
		}
	}
	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = json.NewEncoder(c).Encode(reply)
}

type Client struct {
	path string
	uid  uint32
}

func NewClient(path string, uid uint32) *Client { return &Client{path, uid} }
func (c *Client) call(ctx context.Context, req request) (response, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.path)
	if err != nil {
		return response{}, ErrDenied
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	u := conn.(*net.UnixConn)
	p, err := peer(u)
	if err != nil || p.UID != c.uid {
		return response{}, ErrDenied
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	req.Version = 1
	if json.NewEncoder(conn).Encode(req) != nil {
		return response{}, ErrDenied
	}
	data, err := bufio.NewReader(io.LimitReader(conn, maxFrame+1)).ReadBytes('\n')
	if err != nil || len(data) > maxFrame {
		return response{}, ErrDenied
	}
	var reply response
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&reply) != nil || reply.Error != "" {
		return response{}, ErrDenied
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return response{}, ErrDenied
	}
	return reply, nil
}
func (c *Client) Login(ctx context.Context, user string, password []byte) (Session, error) {
	r, e := c.call(ctx, request{Action: "login", UserID: user, Password: password})
	if e != nil || r.Session == nil {
		return Session{}, ErrDenied
	}
	return *r.Session, nil
}
func (c *Client) Verify(ctx context.Context, token string, p Peer) (Session, error) {
	r, e := c.call(ctx, request{Action: "verify", Token: token, PeerUID: p.UID, PeerPID: p.PID})
	if e != nil || r.Session == nil {
		return Session{}, ErrDenied
	}
	return *r.Session, nil
}
func (c *Client) Logout(ctx context.Context, token string) error {
	_, e := c.call(ctx, request{Action: "logout", Token: token})
	return e
}
