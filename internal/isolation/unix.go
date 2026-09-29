package isolation

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

	"safeterm/internal/contract"
)

const maxIPCMessageBytes = 1 << 20

type ipcReply struct {
	Response *contract.ExecutionResponse `json:"response,omitempty"`
	Error    string                      `json:"error,omitempty"`
}

type UnixServer struct {
	listener   *net.UnixListener
	handler    Client
	allowedUID map[uint32]bool
}

func ListenUnix(path string, mode os.FileMode, allowedUIDs []uint32, handler Client) (*UnixServer, error) {
	if path == "" || handler == nil || len(allowedUIDs) == 0 {
		return nil, fmt.Errorf("isolation: socket path, handler, and allowed UIDs are required")
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("isolation: listen on %q: %w", path, err)
	}
	if err := os.Chmod(path, mode.Perm()); err != nil {
		listener.Close()
		return nil, fmt.Errorf("isolation: secure socket permissions: %w", err)
	}
	allowed := map[uint32]bool{}
	for _, uid := range allowedUIDs {
		allowed[uid] = true
	}
	return &UnixServer{listener: listener, handler: handler, allowedUID: allowed}, nil
}

func (s *UnixServer) Serve(ctx context.Context) error {
	if s == nil || s.listener == nil || ctx == nil {
		return fmt.Errorf("isolation: invalid Unix server")
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			s.listener.Close()
		case <-done:
		}
	}()
	defer close(done)
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("isolation: accept Unix connection: %w", err)
		}
		go s.handle(conn)
	}
}

func (s *UnixServer) Close() error {
	if s == nil || s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *UnixServer) handle(conn *net.UnixConn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	credentials, err := credentialsFor(conn)
	if err != nil || !s.allowedUID[credentials.UID] {
		return
	}
	var request contract.ExecutionRequest
	frame, readErr := bufio.NewReader(io.LimitReader(conn, maxIPCMessageBytes+1)).ReadBytes('\n')
	if readErr != nil || len(frame) > maxIPCMessageBytes {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: "invalid request"})
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		json.NewEncoder(conn).Encode(ipcReply{Error: "invalid request"})
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: "invalid request"})
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	response, err := s.handler.Handle(context.Background(), &request)
	reply := ipcReply{Response: response}
	if err != nil {
		reply.Response, reply.Error = nil, err.Error()
	}
	_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_ = json.NewEncoder(conn).Encode(reply)
}

type UnixClient struct {
	path       string
	allowedUID map[uint32]bool
}

func NewUnixClient(path string, allowedServerUIDs []uint32) (*UnixClient, error) {
	if path == "" || len(allowedServerUIDs) == 0 {
		return nil, fmt.Errorf("isolation: socket path and allowed server UIDs are required")
	}
	allowed := map[uint32]bool{}
	for _, uid := range allowedServerUIDs {
		allowed[uid] = true
	}
	return &UnixClient{path: path, allowedUID: allowed}, nil
}

func (c *UnixClient) Handle(ctx context.Context, request *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	if c == nil || ctx == nil || request == nil {
		return nil, fmt.Errorf("isolation: invalid Unix client request")
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", c.path)
	if err != nil {
		return nil, fmt.Errorf("isolation: connect to Runner: %w", err)
	}
	conn := connection.(*net.UnixConn)
	defer conn.Close()
	credentials, err := credentialsFor(conn)
	if err != nil || !c.allowedUID[credentials.UID] {
		return nil, fmt.Errorf("isolation: Runner peer is not trusted")
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Minute))
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return nil, fmt.Errorf("isolation: send request: %w", err)
	}
	var reply ipcReply
	if err := json.NewDecoder(io.LimitReader(conn, maxIPCMessageBytes)).Decode(&reply); err != nil {
		return nil, fmt.Errorf("isolation: receive response: %w", err)
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("isolation: Runner: %s", reply.Error)
	}
	if reply.Response == nil {
		return nil, fmt.Errorf("isolation: Runner returned no response")
	}
	return reply.Response, nil
}
