package isolation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/siteconfig"
)

type serviceClientFunc func(context.Context, *contract.ExecutionRequest) (*contract.ExecutionResponse, error)

func (f serviceClientFunc) Handle(ctx context.Context, req *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	return f(ctx, req)
}

func TestGatewayBlocksOverlappingGroupsAcrossProfiles(t *testing.T) {
	cfg, err := siteconfig.Prepare(testCatalog(t), siteconfig.Inputs{Profiles: []siteconfig.Named[siteconfig.Profile]{{Value: siteconfig.Profile{Name: "other-services", Base: config.ProfileServiceManager}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		name    string
		profile config.RunnerProfile
		units   []string
	}{
		{"group-a", config.ProfileServiceManager, []string{"one.service", "shared.service"}},
		{"group-b", "other-services", []string{"shared.service", "two.service"}},
		{"group-c", "other-services", []string{"unrelated.service"}},
	} {
		cfg.Operations = append(cfg.Operations, config.OperationDefinition{Name: entry.name, RunnerProfile: entry.profile, ExecutorKind: config.ExecutorSystemd, Systemd: &config.SystemdOperationDefinition{Units: entry.units, Action: "restart"}})
		for i := range cfg.Policies {
			if cfg.Policies[i].Role == "service-operator" {
				cfg.Policies[i].Operations = append(cfg.Policies[i].Operations, entry.name)
			}
		}
	}
	store, err := NewStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	unblock := make(chan struct{})
	first := serviceClientFunc(func(_ context.Context, req *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
		close(entered)
		<-unblock
		return &contract.ExecutionResponse{RequestID: req.RequestID, Result: contract.ResultSuccess}, nil
	})
	second := serviceClientFunc(func(_ context.Context, req *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
		return &contract.ExecutionResponse{RequestID: req.RequestID, Result: contract.ResultSuccess}, nil
	})
	g, err := NewGateway(store, map[config.RunnerProfile]Client{config.ProfileServiceManager: first, "other-services": second})
	if err != nil {
		t.Fatal(err)
	}
	request := func(name string) *contract.ExecutionRequest {
		req, err := contract.NewExecutionRequest(name, nil, contract.Metadata{BrokerType: "ssh", UserID: "alice", ClaimedRoles: []string{"service-operator"}})
		if err != nil {
			t.Fatal(err)
		}
		return req
	}
	done := make(chan error, 1)
	go func() { _, err := g.Dispatch(context.Background(), request("group-a")); done <- err }()
	<-entered
	if _, err := g.Dispatch(context.Background(), request("group-b")); err == nil || !strings.Contains(err.Error(), "conflicts") {
		close(unblock)
		t.Fatalf("overlapping profile accepted: %v", err)
	}
	if _, err := g.Dispatch(context.Background(), request("group-c")); err != nil {
		close(unblock)
		t.Fatalf("unrelated group blocked: %v", err)
	}
	close(unblock)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := g.Dispatch(context.Background(), request("group-b")); err != nil {
		t.Fatal("reservation was not released", err)
	}
	op, _ := store.Current().Operation("group-b")
	op.Systemd.Units[0] = "mutated.service"
	again, _ := store.Current().Operation("group-b")
	if again.Systemd.Units[0] != "shared.service" {
		t.Fatal("snapshot plan aliased caller")
	}
}

func TestGatewayRetainsServiceReservationAfterUncertainTransportFailure(t *testing.T) {
	store, err := NewStore(testCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := serviceClientFunc(func(ctx context.Context, _ *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded service dispatch")
		}
		return nil, errors.New("connection failed after send")
	})
	g, err := NewGateway(store, map[config.RunnerProfile]Client{config.ProfileServiceManager: client})
	if err != nil {
		t.Fatal(err)
	}
	req, err := contract.NewExecutionRequest("restart_safeterm_ssh", nil, contract.Metadata{BrokerType: "ssh", UserID: "alice", ClaimedRoles: []string{"service-operator"}})
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Dispatch(canceled, req); err == nil || calls != 0 {
		t.Fatal("previously canceled service request executed")
	}
	if _, err := g.Dispatch(context.Background(), req); err == nil || calls != 1 {
		t.Fatal("transport error not propagated")
	}
	if release, ok := g.serviceLocks.Acquire([]string{"safeterm-ssh-broker.service"}); ok {
		release()
		t.Fatal("uncertain sequence lost reservation")
	}
}
