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

const (
	maxIPCMessageBytes          = 1 << 20
	maxConcurrentIPCConnections = 128
)

type ipcReply struct {
	Response *contract.ExecutionResponse `json:"response,omitempty"`
	Error    string                      `json:"error,omitempty"`
}

type UnixServer struct {
	listener     *net.UnixListener
	handler      Client
	streaming    StreamingService
	peerPolicies map[uint32]PeerPolicy
	connections  chan struct{}
}

// PeerPolicy binds an authenticated operating-system identity to the IPC
// capabilities it may use. Authorize runs after strict decoding and before a
// unary request crosses the Gateway boundary.
type PeerPolicy struct {
	AllowStreaming   bool
	Authorize        func(*contract.ExecutionRequest) error
	AuthorizePeer    func(*contract.ExecutionRequest, PeerIdentity) error
	AuthorizeControl func(*contract.ControlRequest, PeerIdentity) error
}

type PeerIdentity struct {
	UID uint32
	PID int32
}

func (p PeerPolicy) authorize(req *contract.ExecutionRequest, peer PeerIdentity) error {
	if p.Authorize != nil {
		if err := p.Authorize(req); err != nil {
			return err
		}
	}
	if p.AuthorizePeer != nil {
		return p.AuthorizePeer(req, peer)
	}
	return nil
}

// ListenUnixWithPeerPolicies exposes the same strict bounded unary framing with
// per-peer identity verification; privileged typed services cannot opt into streams.
func ListenUnixWithPeerPolicies(path string, mode os.FileMode, policies map[uint32]PeerPolicy, handler Client) (*UnixServer, error) {
	return listenUnix(path, mode, policies, handler, nil)
}

func ListenUnix(path string, mode os.FileMode, allowedUIDs []uint32, handler Client) (*UnixServer, error) {
	return listenUnix(path, mode, permissivePeerPolicies(allowedUIDs, false), handler, nil)
}

func ListenUnixStreaming(path string, mode os.FileMode, allowedUIDs []uint32, handler Client, streaming StreamingService) (*UnixServer, error) {
	if streaming == nil {
		return nil, fmt.Errorf("isolation: streaming service is required")
	}
	return listenUnix(path, mode, permissivePeerPolicies(allowedUIDs, true), handler, streaming)
}

func ListenUnixStreamingWithPeerPolicies(path string, mode os.FileMode, policies map[uint32]PeerPolicy, handler Client, streaming StreamingService) (*UnixServer, error) {
	if streaming == nil {
		return nil, fmt.Errorf("isolation: streaming service is required")
	}
	return listenUnix(path, mode, policies, handler, streaming)
}

func permissivePeerPolicies(allowedUIDs []uint32, streaming bool) map[uint32]PeerPolicy {
	policies := map[uint32]PeerPolicy{}
	for _, uid := range allowedUIDs {
		policies[uid] = PeerPolicy{AllowStreaming: streaming}
	}
	return policies
}

func listenUnix(path string, mode os.FileMode, policies map[uint32]PeerPolicy, handler Client, streaming StreamingService) (*UnixServer, error) {
	if path == "" || handler == nil || len(policies) == 0 {
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
	copyPolicies := make(map[uint32]PeerPolicy, len(policies))
	for uid, policy := range policies {
		copyPolicies[uid] = policy
	}
	return &UnixServer{
		listener: listener, handler: handler, streaming: streaming,
		peerPolicies: copyPolicies, connections: make(chan struct{}, maxConcurrentIPCConnections),
	}, nil
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
		if !s.acquireConnection() {
			_ = conn.Close()
			continue
		}
		go func() {
			defer s.releaseConnection()
			s.handle(conn)
		}()
	}
}

func (s *UnixServer) acquireConnection() bool {
	select {
	case s.connections <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *UnixServer) releaseConnection() { <-s.connections }

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
	policy, allowed := s.peerPolicies[credentials.UID]
	if err != nil || !allowed {
		return
	}
	var request contract.ExecutionRequest
	frame, readErr := bufio.NewReader(io.LimitReader(conn, maxIPCMessageBytes+1)).ReadBytes('\n')
	if readErr != nil || len(frame) > maxIPCMessageBytes {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: "invalid request"})
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(frame, &probe); err == nil && probe.Type != "" {
		if !policy.AllowStreaming {
			_ = json.NewEncoder(conn).Encode(ipcReply{Error: "streaming is not authorized for peer"})
			return
		}
		s.handleStream(conn, frame, policy, PeerIdentity{credentials.UID, credentials.PID})
		return
	}
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
	{
		if err := policy.authorize(&request, PeerIdentity{credentials.UID, credentials.PID}); err != nil {
			_ = json.NewEncoder(conn).Encode(ipcReply{Error: "peer is not authorized for request"})
			return
		}
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
	stopCancellation := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancellation()
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
