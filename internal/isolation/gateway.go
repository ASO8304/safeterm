package isolation

import (
	"context"
	"fmt"
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

type Gateway struct {
	catalog *Store
	clients map[config.RunnerProfile]Client
	sink    EventSink
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
