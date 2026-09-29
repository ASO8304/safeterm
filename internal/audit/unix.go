package audit

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
)

const maxIPCMessageBytes = 1 << 20

type ipcReply struct {
	Accepted bool   `json:"accepted"`
	Error    string `json:"error,omitempty"`
}

type UnixServer struct {
	listener  *net.UnixListener
	sink      Sink
	producers map[uint32][]Producer
}

type Producer struct {
	Component string
	Profile   string
}

func ListenUnix(path string, mode os.FileMode, producers map[uint32][]Producer, sink Sink) (*UnixServer, error) {
	if path == "" || sink == nil || len(producers) == 0 {
		return nil, fmt.Errorf("audit: socket path, sink, and producer policies are required")
	}
	policies := make(map[uint32][]Producer, len(producers))
	for uid, identities := range producers {
		if len(identities) == 0 {
			return nil, fmt.Errorf("audit: UID %d has no producer policy", uid)
		}
		for _, producer := range identities {
			if producer.Component != ComponentGateway && producer.Component != ComponentRunner {
				return nil, fmt.Errorf("audit: invalid producer component %q", producer.Component)
			}
			if producer.Component == ComponentRunner && producer.Profile == "" {
				return nil, fmt.Errorf("audit: runner producer requires a profile")
			}
		}
		policies[uid] = append([]Producer(nil), identities...)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("audit: listen on %q: %w", path, err)
	}
	if err := os.Chmod(path, mode.Perm()); err != nil {
		listener.Close()
		return nil, fmt.Errorf("audit: secure socket permissions: %w", err)
	}
	return &UnixServer{listener: listener, sink: sink, producers: policies}, nil
}

func (s *UnixServer) Serve(ctx context.Context) error {
	if s == nil || s.listener == nil || ctx == nil {
		return fmt.Errorf("audit: invalid Unix server")
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
			return fmt.Errorf("audit: accept Unix connection: %w", err)
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
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	credentials, err := credentialsFor(conn)
	producers, ok := s.producers[credentials.UID]
	if err != nil || !ok {
		return
	}
	frame, readErr := bufio.NewReader(io.LimitReader(conn, maxIPCMessageBytes+1)).ReadBytes('\n')
	if readErr != nil || len(frame) > maxIPCMessageBytes {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: "invalid event"})
		return
	}
	var event Event
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: "invalid event"})
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: "invalid event"})
		return
	}
	authorized := false
	for _, producer := range producers {
		if event.Component == producer.Component && (producer.Component != ComponentRunner || event.Profile == producer.Profile) {
			authorized = true
			break
		}
	}
	if !authorized {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: "producer is not authorized for event source"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.sink.Record(ctx, event); err != nil {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: err.Error()})
		return
	}
	_ = json.NewEncoder(conn).Encode(ipcReply{Accepted: true})
}

type UnixClient struct {
	path       string
	allowedUID map[uint32]bool
}

func NewUnixClient(path string, allowedServerUIDs []uint32) (*UnixClient, error) {
	if path == "" || len(allowedServerUIDs) == 0 {
		return nil, fmt.Errorf("audit: socket path and allowed server UIDs are required")
	}
	allowed := map[uint32]bool{}
	for _, uid := range allowedServerUIDs {
		allowed[uid] = true
	}
	return &UnixClient{path: path, allowedUID: allowed}, nil
}

func (c *UnixClient) Record(ctx context.Context, event Event) error {
	if c == nil || ctx == nil {
		return fmt.Errorf("audit: invalid Unix client")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", c.path)
	if err != nil {
		return fmt.Errorf("audit: connect to service: %w", err)
	}
	conn := connection.(*net.UnixConn)
	defer conn.Close()
	credentials, err := credentialsFor(conn)
	if err != nil || !c.allowedUID[credentials.UID] {
		return fmt.Errorf("audit: service peer is not trusted")
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}
	if err := json.NewEncoder(conn).Encode(event); err != nil {
		return fmt.Errorf("audit: send event: %w", err)
	}
	var reply ipcReply
	if err := json.NewDecoder(io.LimitReader(conn, maxIPCMessageBytes)).Decode(&reply); err != nil {
		return fmt.Errorf("audit: receive acknowledgement: %w", err)
	}
	if !reply.Accepted {
		return fmt.Errorf("audit: service rejected event: %s", reply.Error)
	}
	return nil
}
