package isolation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"safeterm/internal/contract"
)

const maxStreamFrameBytes = 128 << 10

const (
	ipcTypeStart     = "start"
	ipcTypeTerminate = "terminate"
)

type streamIPCRequest struct {
	Version int                        `json:"version"`
	Type    string                     `json:"type"`
	Request *contract.ExecutionRequest `json:"request,omitempty"`
	Control *contract.ControlRequest   `json:"control,omitempty"`
}

type streamIPCReply struct {
	Frame   *contract.StreamFrame   `json:"frame,omitempty"`
	Control *contract.ControlResult `json:"control,omitempty"`
	Error   string                  `json:"error,omitempty"`
}

type streamDisconnector interface {
	disconnect(context.Context, string, contract.SessionOwner)
}

func (s *UnixServer) handleStream(conn *net.UnixConn, data []byte) {
	if s.streaming == nil {
		_ = json.NewEncoder(conn).Encode(streamIPCReply{Error: "streaming is unavailable"})
		return
	}
	var request streamIPCRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.Version != contract.StreamProtocolVersion {
		_ = json.NewEncoder(conn).Encode(streamIPCReply{Error: "invalid stream request"})
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		_ = json.NewEncoder(conn).Encode(streamIPCReply{Error: "invalid stream request"})
		return
	}
	switch request.Type {
	case ipcTypeStart:
		if request.Request == nil || request.Control != nil {
			_ = json.NewEncoder(conn).Encode(streamIPCReply{Error: "invalid start request"})
			return
		}
		_ = conn.SetReadDeadline(time.Time{})
		session, err := s.streaming.StartStream(context.Background(), request.Request)
		if err != nil {
			_ = json.NewEncoder(conn).Encode(streamIPCReply{Error: err.Error()})
			return
		}
		owner := contract.OwnerFromMetadata(request.Request.Metadata)
		disconnected := make(chan struct{})
		go func() {
			var buffer [1]byte
			_, _ = conn.Read(buffer[:])
			close(disconnected)
		}()
		for {
			var frame contract.StreamFrame
			var ok bool
			select {
			case frame, ok = <-session.Events:
				if !ok {
					return
				}
			case <-disconnected:
				if disconnector, ok := s.streaming.(streamDisconnector); ok {
					disconnector.disconnect(context.Background(), session.ID, owner)
				}
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := json.NewEncoder(conn).Encode(streamIPCReply{Frame: &frame}); err != nil {
				if disconnector, ok := s.streaming.(streamDisconnector); ok {
					disconnector.disconnect(context.Background(), session.ID, owner)
				}
				return
			}
		}
	case ipcTypeTerminate:
		if request.Control == nil || request.Request != nil {
			_ = json.NewEncoder(conn).Encode(streamIPCReply{Error: "invalid terminate request"})
			return
		}
		result, err := s.streaming.Terminate(context.Background(), *request.Control)
		if err != nil {
			_ = json.NewEncoder(conn).Encode(streamIPCReply{Error: err.Error()})
			return
		}
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = json.NewEncoder(conn).Encode(streamIPCReply{Control: &result})
	default:
		_ = json.NewEncoder(conn).Encode(streamIPCReply{Error: "unsupported stream request"})
	}
}

func (c *UnixClient) StartStream(ctx context.Context, request *contract.ExecutionRequest) (*StreamSession, error) {
	if c == nil || ctx == nil || request == nil {
		return nil, fmt.Errorf("isolation: invalid Unix streaming request")
	}
	conn, err := c.dialTrusted(ctx)
	if err != nil {
		return nil, err
	}
	// Make cancellation effective during the start handshake as well.
	stopCancellation := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancellation()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
	} else {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	envelope := streamIPCRequest{Version: contract.StreamProtocolVersion, Type: ipcTypeStart, Request: request}
	if err := json.NewEncoder(conn).Encode(envelope); err != nil {
		conn.Close()
		return nil, fmt.Errorf("isolation: send stream start: %w", err)
	}
	_ = conn.SetWriteDeadline(time.Time{})
	reader := bufio.NewReaderSize(conn, maxStreamFrameBytes+1)
	var first streamIPCReply
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	if err := readBoundedJSONLine(reader, maxStreamFrameBytes, &first); err != nil {
		conn.Close()
		return nil, fmt.Errorf("isolation: receive session start: %w", err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	if first.Error != "" || first.Frame == nil || first.Frame.Type != contract.FrameSessionStarted || first.Frame.RequestID != request.RequestID {
		conn.Close()
		if first.Error != "" {
			return nil, fmt.Errorf("isolation: Runner: %s", first.Error)
		}
		return nil, fmt.Errorf("isolation: Runner returned no session start")
	}
	if err := first.Frame.Validate(contract.MaxStreamChunkBytes); err != nil {
		conn.Close()
		return nil, err
	}
	events := make(chan contract.StreamFrame, 64)
	events <- *first.Frame
	go func() {
		defer close(events)
		defer conn.Close()
		closed := make(chan struct{})
		defer close(closed)
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-closed:
			}
		}()
		for {
			var reply streamIPCReply
			if err := readBoundedJSONLine(reader, maxStreamFrameBytes, &reply); err != nil {
				return
			}
			if reply.Error != "" || reply.Frame == nil {
				return
			}
			if err := reply.Frame.Validate(contract.MaxStreamChunkBytes); err != nil {
				return
			}
			if reply.Frame.RequestID != first.Frame.RequestID || reply.Frame.SessionID != first.Frame.SessionID {
				return
			}
			select {
			case events <- *reply.Frame:
			case <-ctx.Done():
				return
			}
			if reply.Frame.Type == contract.FrameSessionExited {
				return
			}
		}
	}()
	return &StreamSession{ID: first.Frame.SessionID, RequestID: first.Frame.RequestID, Events: events}, nil
}

func (c *UnixClient) Terminate(ctx context.Context, request contract.ControlRequest) (contract.ControlResult, error) {
	if c == nil || ctx == nil {
		return contract.ControlResult{}, fmt.Errorf("isolation: invalid Unix control request")
	}
	if err := request.Validate(); err != nil {
		return contract.ControlResult{}, err
	}
	conn, err := c.dialTrusted(ctx)
	if err != nil {
		return contract.ControlResult{}, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}
	envelope := streamIPCRequest{Version: contract.StreamProtocolVersion, Type: ipcTypeTerminate, Control: &request}
	if err := json.NewEncoder(conn).Encode(envelope); err != nil {
		return contract.ControlResult{}, fmt.Errorf("isolation: send terminate: %w", err)
	}
	reader := bufio.NewReaderSize(conn, maxStreamFrameBytes+1)
	var reply streamIPCReply
	if err := readBoundedJSONLine(reader, maxStreamFrameBytes, &reply); err != nil {
		return contract.ControlResult{}, fmt.Errorf("isolation: receive terminate result: %w", err)
	}
	if reply.Error != "" || reply.Control == nil {
		if reply.Error != "" {
			return contract.ControlResult{}, fmt.Errorf("isolation: Runner: %s", reply.Error)
		}
		return contract.ControlResult{}, fmt.Errorf("isolation: Runner returned no control result")
	}
	if err := reply.Control.Validate(); err != nil {
		return contract.ControlResult{}, err
	}
	if reply.Control.SessionID != request.SessionID {
		return contract.ControlResult{}, fmt.Errorf("isolation: Runner returned a mismatched session ID")
	}
	return *reply.Control, nil
}

func (c *UnixClient) dialTrusted(ctx context.Context) (*net.UnixConn, error) {
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", c.path)
	if err != nil {
		return nil, fmt.Errorf("isolation: connect to Runner: %w", err)
	}
	conn := connection.(*net.UnixConn)
	credentials, err := credentialsFor(conn)
	if err != nil || !c.allowedUID[credentials.UID] {
		conn.Close()
		return nil, fmt.Errorf("isolation: Runner peer is not trusted")
	}
	return conn, nil
}

func readBoundedJSONLine(reader *bufio.Reader, limit int, destination any) error {
	line, err := reader.ReadSlice('\n')
	if err != nil {
		if err == bufio.ErrBufferFull {
			return fmt.Errorf("frame exceeds %d bytes", limit)
		}
		return err
	}
	if len(line) > limit {
		return fmt.Errorf("frame exceeds %d bytes", limit)
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("frame has trailing data")
	}
	return nil
}
