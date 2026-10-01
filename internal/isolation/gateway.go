package isolation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"safeterm/internal/audit"
	"safeterm/internal/config"
	"safeterm/internal/contract"
)

// Client is a transport boundary to one isolated Runner service. Gateway has
// deliberately no agent.Executor and cannot execute a host command itself.
type Client interface {
	Handle(ctx context.Context, req *contract.ExecutionRequest) (*contract.ExecutionResponse, error)
}

type StreamingClient interface {
	StartStream(context.Context, *contract.ExecutionRequest) (*StreamSession, error)
	Terminate(context.Context, contract.ControlRequest) (contract.ControlResult, error)
}

type sessionRoute struct {
	client  StreamingClient
	owner   contract.SessionOwner
	profile config.RunnerProfile
}

type Gateway struct {
	catalog   *Store
	clients   map[config.RunnerProfile]Client
	sink      EventSink
	sessionMu sync.RWMutex
	sessions  map[string]sessionRoute
}

type LifecycleEvent = audit.Event
type EventSink = audit.Sink

func NewGateway(catalog *Store, clients map[config.RunnerProfile]Client) (*Gateway, error) {
	return NewGatewayWithSink(catalog, clients, nil)
}

func NewGatewayWithSink(catalog *Store, clients map[config.RunnerProfile]Client, sink EventSink) (*Gateway, error) {
	if catalog == nil || catalog.Current() == nil {
		return nil, fmt.Errorf("isolation: gateway requires a catalog")
	}
	copyClients := make(map[config.RunnerProfile]Client, len(clients))
	for profile, client := range clients {
		if client == nil {
			return nil, fmt.Errorf("isolation: nil client for profile %q", profile)
		}
		if _, ok := catalog.Current().Profile(profile); !ok {
			return nil, fmt.Errorf("isolation: client references unknown profile %q", profile)
		}
		copyClients[profile] = client
	}
	return &Gateway{catalog: catalog, clients: copyClients, sink: sink, sessions: map[string]sessionRoute{}}, nil
}

// StartStream routes an approved long-running operation to exactly one Runner
// and records the opaque session ownership for later semantic control.
func (g *Gateway) StartStream(ctx context.Context, req *contract.ExecutionRequest) (*StreamSession, error) {
	if g == nil || g.catalog == nil || ctx == nil || req == nil {
		return nil, fmt.Errorf("isolation: invalid gateway streaming request")
	}
	snapshot := g.catalog.Current()
	operation, ok := snapshot.Operation(req.Command)
	if !ok {
		return nil, fmt.Errorf("isolation: unknown operation %q", req.Command)
	}
	if !operation.LongRunning {
		return nil, fmt.Errorf("isolation: operation %q is not long-running", req.Command)
	}
	owner := contract.OwnerFromMetadata(req.Metadata)
	if owner.BrokerType == "" || owner.UserID == "" {
		return nil, fmt.Errorf("isolation: streaming session requires broker and user ownership claims")
	}
	client, ok := g.clients[operation.RunnerProfile]
	if !ok {
		return nil, fmt.Errorf("isolation: profile %q is unavailable", operation.RunnerProfile)
	}
	streaming, ok := client.(StreamingClient)
	if !ok {
		return nil, fmt.Errorf("isolation: profile %q does not support streaming", operation.RunnerProfile)
	}
	if err := g.record(ctx, req, operation.RunnerProfile, "routed", snapshot.Version(), nil); err != nil {
		return nil, err
	}
	session, err := streaming.StartStream(ctx, req)
	if err != nil {
		return nil, err
	}
	if session == nil || session.ID == "" || session.RequestID != req.RequestID || session.Events == nil {
		return nil, fmt.Errorf("isolation: Runner returned an invalid streaming session")
	}
	g.sessionMu.Lock()
	g.sessions[session.ID] = sessionRoute{client: streaming, owner: owner, profile: operation.RunnerProfile}
	g.sessionMu.Unlock()
	forwarded := make(chan contract.StreamFrame, 64)
	go func() {
		defer close(forwarded)
		defer time.AfterFunc(5*time.Minute, func() {
			g.sessionMu.Lock()
			delete(g.sessions, session.ID)
			g.sessionMu.Unlock()
		})
		for frame := range session.Events {
			select {
			case forwarded <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	return &StreamSession{ID: session.ID, RequestID: session.RequestID, Events: forwarded}, nil
}

// Terminate routes only the semantic action to the Runner that created the
// session. No PID, signal, argv, or profile selection crosses this boundary.
func (g *Gateway) Terminate(ctx context.Context, request contract.ControlRequest) (contract.ControlResult, error) {
	if g == nil || ctx == nil {
		return contract.ControlResult{}, fmt.Errorf("isolation: invalid gateway control request")
	}
	if err := request.Validate(); err != nil {
		return contract.ControlResult{}, err
	}
	g.sessionMu.RLock()
	route, ok := g.sessions[request.SessionID]
	g.sessionMu.RUnlock()
	if !ok {
		return contract.ControlResult{Version: contract.StreamProtocolVersion, SessionID: request.SessionID, Status: contract.ControlNotFound}, nil
	}
	if route.owner != request.Owner {
		return contract.ControlResult{Version: contract.StreamProtocolVersion, SessionID: request.SessionID, Status: contract.ControlDenied}, nil
	}
	return route.client.Terminate(ctx, request)
}

func (g *Gateway) Dispatch(ctx context.Context, req *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	if g == nil || g.catalog == nil {
		return nil, fmt.Errorf("isolation: unconfigured gateway")
	}
	if ctx == nil || req == nil {
		return nil, fmt.Errorf("isolation: context and request are required")
	}
	snapshot := g.catalog.Current()
	profile, ok := snapshot.Route(req.Command)
	if !ok {
		if err := g.record(ctx, req, "", "unknown-operation", snapshot.Version(), nil); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("isolation: unknown operation %q", req.Command)
	}
	client, ok := g.clients[profile]
	if !ok {
		if err := g.record(ctx, req, profile, "profile-unavailable", snapshot.Version(), nil); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("isolation: profile %q is unavailable", profile)
	}
	if err := g.record(ctx, req, profile, "routed", snapshot.Version(), nil); err != nil {
		return nil, err
	}
	response, err := client.Handle(ctx, req)
	if err != nil {
		if recordErr := g.record(ctx, req, profile, "runner-error", snapshot.Version(), nil); recordErr != nil {
			return nil, recordErr
		}
		return nil, err
	}
	if err := g.record(ctx, req, profile, "returned", snapshot.Version(), response); err != nil {
		return nil, err
	}
	return response, nil
}

func (g *Gateway) record(ctx context.Context, req *contract.ExecutionRequest, profile config.RunnerProfile, outcome, version string, response *contract.ExecutionResponse) error {
	if g.sink == nil {
		return nil
	}
	if err := g.sink.Record(ctx, audit.NewEvent(req, version, audit.ComponentGateway, string(profile), outcome, response)); err != nil {
		return fmt.Errorf("isolation: audit gateway event: %w", err)
	}
	return nil
}

// Handle lets Gateway serve the same bounded IPC protocol as a Runner while
// retaining Dispatch as the explicit routing API.
func (g *Gateway) Handle(ctx context.Context, req *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	return g.Dispatch(ctx, req)
}

type RequestHandler interface {
	HandleRequest(ctx context.Context, req *contract.ExecutionRequest) *contract.ExecutionResponse
}

// ProfileHandler is the final local gate in front of one Agent Core.
type ProfileHandler struct {
	profile config.RunnerProfile
	catalog *Store
	core    RequestHandler
	sink    EventSink
}

func NewProfileHandler(profile config.RunnerProfile, catalog *Store, core RequestHandler) (*ProfileHandler, error) {
	return NewProfileHandlerWithSink(profile, catalog, core, nil)
}

func NewProfileHandlerWithSink(profile config.RunnerProfile, catalog *Store, core RequestHandler, sink EventSink) (*ProfileHandler, error) {
	if catalog == nil || catalog.Current() == nil || core == nil {
		return nil, fmt.Errorf("isolation: profile handler requires catalog and core")
	}
	if _, ok := catalog.Current().Profile(profile); !ok {
		return nil, fmt.Errorf("isolation: unknown profile %q", profile)
	}
	return &ProfileHandler{profile: profile, catalog: catalog, core: core, sink: sink}, nil
}

func (h *ProfileHandler) Handle(ctx context.Context, req *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	if h == nil || h.catalog == nil || h.core == nil || ctx == nil || req == nil {
		return nil, fmt.Errorf("isolation: invalid profile request")
	}
	snapshot := h.catalog.Current()
	if !snapshot.Allows(h.profile, req.Command) {
		now := time.Now().UTC()
		response := &contract.ExecutionResponse{RequestID: req.RequestID, Result: contract.ResultRejected, FailureStage: "profile", Message: fmt.Sprintf("isolation: operation %q is not assigned to profile %q", req.Command, h.profile), StartedAt: now, EndedAt: now}
		if err := h.record(ctx, req, "profile-rejected", snapshot.Version(), response); err != nil {
			return nil, err
		}
		return response, nil
	}
	if err := h.record(ctx, req, "accepted", snapshot.Version(), nil); err != nil {
		return nil, err
	}
	response := h.core.HandleRequest(ctx, req)
	if err := h.record(ctx, req, "completed", snapshot.Version(), response); err != nil {
		return nil, err
	}
	return response, nil
}

func (h *ProfileHandler) record(ctx context.Context, req *contract.ExecutionRequest, outcome, version string, response *contract.ExecutionResponse) error {
	if h.sink == nil {
		return nil
	}
	if err := h.sink.Record(ctx, audit.NewEvent(req, version, audit.ComponentRunner, string(h.profile), outcome, response)); err != nil {
		return fmt.Errorf("isolation: audit runner event: %w", err)
	}
	return nil
}
