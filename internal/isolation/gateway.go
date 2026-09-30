package isolation

import (
	"context"
	"fmt"
	"time"

	"safeterm/internal/config"
	"safeterm/internal/contract"
)

// Client is a transport boundary to one isolated Runner service. Gateway has
// deliberately no agent.Executor and cannot execute a host command itself.
type Client interface {
	Handle(ctx context.Context, req *contract.ExecutionRequest) (*contract.ExecutionResponse, error)
}

type Gateway struct {
	catalog *Store
	clients map[config.RunnerProfile]Client
	sink    EventSink
}

type LifecycleEvent struct {
	RequestID      string
	CatalogVersion string
	Component      string
	Profile        config.RunnerProfile
	Operation      string
	Outcome        string
	Timestamp      time.Time
}

type EventSink interface{ Record(LifecycleEvent) }

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
	return &Gateway{catalog: catalog, clients: copyClients, sink: sink}, nil
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
		g.record(req, "", "unknown-operation", snapshot.Version())
		return nil, fmt.Errorf("isolation: unknown operation %q", req.Command)
	}
	client, ok := g.clients[profile]
	if !ok {
		g.record(req, profile, "profile-unavailable", snapshot.Version())
		return nil, fmt.Errorf("isolation: profile %q is unavailable", profile)
	}
	g.record(req, profile, "routed", snapshot.Version())
	return client.Handle(ctx, req)
}

func (g *Gateway) record(req *contract.ExecutionRequest, profile config.RunnerProfile, outcome, version string) {
	if g.sink == nil {
		return
	}
	g.sink.Record(LifecycleEvent{RequestID: req.RequestID, CatalogVersion: version, Component: "gateway", Profile: profile, Operation: req.Command, Outcome: outcome, Timestamp: time.Now().UTC()})
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
		h.record(req, "profile-rejected", snapshot.Version())
		now := time.Now().UTC()
		return &contract.ExecutionResponse{RequestID: req.RequestID, Result: contract.ResultRejected, Message: fmt.Sprintf("isolation: operation %q is not assigned to profile %q", req.Command, h.profile), StartedAt: now, EndedAt: now}, nil
	}
	h.record(req, "accepted", snapshot.Version())
	return h.core.HandleRequest(ctx, req), nil
}

func (h *ProfileHandler) record(req *contract.ExecutionRequest, outcome, version string) {
	if h.sink == nil {
		return
	}
	h.sink.Record(LifecycleEvent{RequestID: req.RequestID, CatalogVersion: version, Component: "runner", Profile: h.profile, Operation: req.Command, Outcome: outcome, Timestamp: time.Now().UTC()})
}
