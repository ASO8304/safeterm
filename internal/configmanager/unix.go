package configmanager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

const maxIPCMessageBytes = 1 << 20

type Service interface {
	List(context.Context, Actor) ([]ResourceSummary, error)
	Describe(context.Context, Actor, string) (ResourceDescription, error)
	Read(context.Context, Actor, string) (ReadResult, error)
	ValidatePatch(context.Context, Actor, Patch) (ValidationResult, error)
	Preview(context.Context, Actor, Patch) (PreviewResult, error)
	Apply(context.Context, Actor, string) (ApplyResult, error)
	Rollback(context.Context, Actor, string) (RollbackResult, error)
}

type ipcRequest struct {
	Version    int    `json:"version"`
	Action     string `json:"action"`
	Actor      Actor  `json:"actor"`
	ResourceID string `json:"resource_id,omitempty"`
	Patch      *Patch `json:"patch,omitempty"`
	Token      string `json:"token,omitempty"`
}

type ipcReply struct {
	Resources   []ResourceSummary    `json:"resources,omitempty"`
	Description *ResourceDescription `json:"description,omitempty"`
	Read        *ReadResult          `json:"read,omitempty"`
	Validation  *ValidationResult    `json:"validation,omitempty"`
	Preview     *PreviewResult       `json:"preview,omitempty"`
	Apply       *ApplyResult         `json:"apply,omitempty"`
	Rollback    *RollbackResult      `json:"rollback,omitempty"`
	Error       string               `json:"error,omitempty"`
}

type UnixServer struct {
	listener *net.UnixListener
	service  Service
	allowed  map[uint32]bool
}

func ListenUnix(path string, mode os.FileMode, allowedUIDs []uint32, service Service) (*UnixServer, error) {
	if path == "" || service == nil || len(allowedUIDs) == 0 {
		return nil, fmt.Errorf("configmanager: socket, service, and allowed UIDs are required")
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("configmanager: listen: %w", err)
	}
	if err := os.Chmod(path, mode.Perm()); err != nil {
		listener.Close()
		return nil, err
	}
	allowed := map[uint32]bool{}
	for _, uid := range allowedUIDs {
		allowed[uid] = true
	}
	return &UnixServer{listener: listener, service: service, allowed: allowed}, nil
}

func (s *UnixServer) Serve(ctx context.Context) error {
	if s == nil || ctx == nil {
		return fmt.Errorf("configmanager: invalid Unix server")
	}
	go func() { <-ctx.Done(); _ = s.listener.Close() }()
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
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
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	uid, err := peerUID(conn)
	if err != nil || !s.allowed[uid] {
		return
	}
	frame, err := bufio.NewReader(io.LimitReader(conn, maxIPCMessageBytes+1)).ReadBytes('\n')
	if err != nil || len(frame) > maxIPCMessageBytes {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: "invalid request"})
		return
	}
	var request ipcRequest
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || request.Version != ProtocolVersion {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: "invalid request"})
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		_ = json.NewEncoder(conn).Encode(ipcReply{Error: "invalid request"})
		return
	}
	reply := s.dispatch(request)
	_ = json.NewEncoder(conn).Encode(reply)
}

func (s *UnixServer) dispatch(request ipcRequest) ipcReply {
	if err := validateIPCRequestShape(request); err != nil {
		return ipcReply{Error: "invalid request shape"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	var reply ipcReply
	var err error
	switch request.Action {
	case "list":
		reply.Resources, err = s.service.List(ctx, request.Actor)
	case "describe":
		var value ResourceDescription
		value, err = s.service.Describe(ctx, request.Actor, request.ResourceID)
		reply.Description = &value
	case "read":
		var value ReadResult
		value, err = s.service.Read(ctx, request.Actor, request.ResourceID)
		reply.Read = &value
	case "validate":
		if request.Patch == nil {
			err = fmt.Errorf("patch is required")
			break
		}
		var value ValidationResult
		value, err = s.service.ValidatePatch(ctx, request.Actor, *request.Patch)
		reply.Validation = &value
	case "preview":
		if request.Patch == nil {
			err = fmt.Errorf("patch is required")
			break
		}
		var value PreviewResult
		value, err = s.service.Preview(ctx, request.Actor, *request.Patch)
		reply.Preview = &value
	case "apply":
		var value ApplyResult
		value, err = s.service.Apply(ctx, request.Actor, request.Token)
		reply.Apply = &value
	case "rollback":
		var value RollbackResult
		value, err = s.service.Rollback(ctx, request.Actor, request.ResourceID)
		reply.Rollback = &value
	default:
		err = fmt.Errorf("unsupported action")
	}
	if err != nil {
		return ipcReply{Error: err.Error()}
	}
	return reply
}

func validateIPCRequestShape(request ipcRequest) error {
	if err := validateActor(request.Actor); err != nil {
		return err
	}
	switch request.Action {
	case "list":
		if request.ResourceID != "" || request.Patch != nil || request.Token != "" {
			return fmt.Errorf("invalid list request")
		}
	case "describe", "read", "rollback":
		if !identifierPattern.MatchString(request.ResourceID) || request.Patch != nil || request.Token != "" {
			return fmt.Errorf("invalid resource request")
		}
	case "validate", "preview":
		if request.Patch == nil || request.ResourceID != "" || request.Token != "" || !identifierPattern.MatchString(request.Patch.ResourceID) {
			return fmt.Errorf("invalid patch request")
		}
	case "apply":
		if request.ResourceID != "" || request.Patch != nil || len(request.Token) != 64 {
			return fmt.Errorf("invalid apply request")
		}
		if _, err := hex.DecodeString(request.Token); err != nil {
			return fmt.Errorf("invalid apply token")
		}
	default:
		return fmt.Errorf("unsupported action")
	}
	return nil
}

type UnixClient struct {
	path    string
	allowed map[uint32]bool
}

func NewUnixClient(path string, allowedUIDs []uint32) (*UnixClient, error) {
	if path == "" || len(allowedUIDs) == 0 {
		return nil, fmt.Errorf("configmanager: socket and server UIDs are required")
	}
	allowed := map[uint32]bool{}
	for _, uid := range allowedUIDs {
		allowed[uid] = true
	}
	return &UnixClient{path: path, allowed: allowed}, nil
}

func (c *UnixClient) call(ctx context.Context, request ipcRequest) (ipcReply, error) {
	if c == nil || ctx == nil {
		return ipcReply{}, fmt.Errorf("configmanager: invalid client")
	}
	request.Version = ProtocolVersion
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", c.path)
	if err != nil {
		return ipcReply{}, fmt.Errorf("configmanager: connect: %w", err)
	}
	conn := connection.(*net.UnixConn)
	defer conn.Close()
	uid, err := peerUID(conn)
	if err != nil || !c.allowed[uid] {
		return ipcReply{}, fmt.Errorf("configmanager: server peer is not trusted")
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return ipcReply{}, err
	}
	var reply ipcReply
	if err := json.NewDecoder(io.LimitReader(conn, maxIPCMessageBytes)).Decode(&reply); err != nil {
		return ipcReply{}, err
	}
	if reply.Error != "" {
		return ipcReply{}, fmt.Errorf("configmanager: service: %s", reply.Error)
	}
	return reply, nil
}

func (c *UnixClient) List(ctx context.Context, actor Actor) ([]ResourceSummary, error) {
	r, e := c.call(ctx, ipcRequest{Action: "list", Actor: actor})
	return r.Resources, e
}
func (c *UnixClient) Describe(ctx context.Context, actor Actor, id string) (ResourceDescription, error) {
	r, e := c.call(ctx, ipcRequest{Action: "describe", Actor: actor, ResourceID: id})
	if r.Description == nil {
		return ResourceDescription{}, nonNilReplyError(e)
	}
	return *r.Description, e
}
func (c *UnixClient) Read(ctx context.Context, actor Actor, id string) (ReadResult, error) {
	r, e := c.call(ctx, ipcRequest{Action: "read", Actor: actor, ResourceID: id})
	if r.Read == nil {
		return ReadResult{}, nonNilReplyError(e)
	}
	return *r.Read, e
}
func (c *UnixClient) ValidatePatch(ctx context.Context, actor Actor, patch Patch) (ValidationResult, error) {
	r, e := c.call(ctx, ipcRequest{Action: "validate", Actor: actor, Patch: &patch})
	if r.Validation == nil {
		return ValidationResult{}, nonNilReplyError(e)
	}
	return *r.Validation, e
}
func (c *UnixClient) Preview(ctx context.Context, actor Actor, patch Patch) (PreviewResult, error) {
	r, e := c.call(ctx, ipcRequest{Action: "preview", Actor: actor, Patch: &patch})
	if r.Preview == nil {
		return PreviewResult{}, nonNilReplyError(e)
	}
	return *r.Preview, e
}
func (c *UnixClient) Apply(ctx context.Context, actor Actor, token string) (ApplyResult, error) {
	r, e := c.call(ctx, ipcRequest{Action: "apply", Actor: actor, Token: token})
	if r.Apply == nil {
		return ApplyResult{}, nonNilReplyError(e)
	}
	return *r.Apply, e
}
func (c *UnixClient) Rollback(ctx context.Context, actor Actor, id string) (RollbackResult, error) {
	r, e := c.call(ctx, ipcRequest{Action: "rollback", Actor: actor, ResourceID: id})
	if r.Rollback == nil {
		return RollbackResult{}, nonNilReplyError(e)
	}
	return *r.Rollback, e
}
func nonNilReplyError(err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("configmanager: service returned no result")
}
