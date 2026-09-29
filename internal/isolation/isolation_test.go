package isolation

import (
	"context"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/contract"
)

func testCatalog(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TEST-ISOLATION-002, TEST-ISOLATION-004
func TestSnapshotRoutesAndFiltersProfiles(t *testing.T) {
	snapshot, err := NewSnapshot(testCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	if profile, ok := snapshot.Route("trace"); !ok || profile != config.ProfileNetworkObserver {
		t.Fatalf("trace route = %q, %v", profile, ok)
	}
	if profile, ok := snapshot.Route("system_info"); !ok || profile != config.ProfileDiagnostics {
		t.Fatalf("system_info route = %q, %v", profile, ok)
	}
	if _, ok := snapshot.Route("missing"); ok {
		t.Fatal("unknown operation received a route")
	}
	filtered, err := snapshot.ProfileConfig(config.ProfileDiagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Profiles) != 1 || len(filtered.Operations) != 1 || filtered.Operations[0].Name != "system_info" || len(filtered.Executables) != 1 {
		t.Fatalf("unexpected diagnostics snapshot: %+v", filtered)
	}
	filtered.Operations[0].Name = "mutated"
	second, _ := snapshot.ProfileConfig(config.ProfileDiagnostics)
	if second.Operations[0].Name != "system_info" {
		t.Fatal("profile config aliased active snapshot")
	}
}

// TEST-ISOLATION-009
func TestStoreActivationIsAtomicAndFailClosed(t *testing.T) {
	store, err := NewStore(testCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	previous := store.Current()
	invalid := testCatalog(t)
	invalid.Version = ""
	if err := store.Activate(invalid); err == nil {
		t.Fatal("invalid snapshot activated")
	}
	if store.Current() != previous || store.Current().Version() != "1.0.0" {
		t.Fatal("failed activation changed current snapshot")
	}
	next := testCatalog(t)
	next.Version = "1.1.0"
	if err := store.Activate(next); err != nil {
		t.Fatal(err)
	}
	if store.Current() == previous || store.Current().Version() != "1.1.0" {
		t.Fatal("valid snapshot was not atomically activated")
	}
}

type fakeClient struct {
	calls    int
	response *contract.ExecutionResponse
}

type eventRecorder struct{ events []LifecycleEvent }

func (r *eventRecorder) Record(event LifecycleEvent) { r.events = append(r.events, event) }

func (c *fakeClient) Handle(_ context.Context, _ *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	c.calls++
	return c.response, nil
}

// TEST-ISOLATION-006
func TestGatewayRoutesWithoutExecuting(t *testing.T) {
	store, _ := NewStore(testCatalog(t))
	diagnostics := &fakeClient{response: &contract.ExecutionResponse{Result: contract.ResultSuccess}}
	network := &fakeClient{response: &contract.ExecutionResponse{Result: contract.ResultSuccess}}
	gateway, err := NewGateway(store, map[config.RunnerProfile]Client{config.ProfileDiagnostics: diagnostics, config.ProfileNetworkObserver: network})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := contract.NewExecutionRequest("trace", map[string]string{"port": "53", "host": "127.0.0.1"}, contract.Metadata{ClaimedRoles: []string{"netops"}})
	if _, err := gateway.Dispatch(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if diagnostics.calls != 0 || network.calls != 1 {
		t.Fatalf("calls: diagnostics=%d network=%d", diagnostics.calls, network.calls)
	}
	request.Command = "missing"
	if _, err := gateway.Dispatch(context.Background(), request); err == nil {
		t.Fatal("unknown operation was routed")
	}
}

type fakeCore struct{ calls int }

func (c *fakeCore) HandleRequest(_ context.Context, req *contract.ExecutionRequest) *contract.ExecutionResponse {
	c.calls++
	return &contract.ExecutionResponse{RequestID: req.RequestID, Result: contract.ResultSuccess}
}

// TEST-ISOLATION-004, TEST-ISOLATION-010, TEST-ISOLATION-011
func TestProfileHandlerRejectsCrossProfileBeforeCore(t *testing.T) {
	store, _ := NewStore(testCatalog(t))
	core := &fakeCore{}
	handler, err := NewProfileHandler(config.ProfileDiagnostics, store, core)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := contract.NewExecutionRequest("trace", nil, contract.Metadata{})
	response, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if core.calls != 0 || response.Result != contract.ResultRejected || response.RequestID != request.RequestID || !strings.Contains(response.Message, "not assigned") {
		t.Fatalf("cross-profile response=%+v calls=%d", response, core.calls)
	}
	request.Command = "system_info"
	response, err = handler.Handle(context.Background(), request)
	if err != nil || response.Result != contract.ResultSuccess || core.calls != 1 {
		t.Fatalf("allowed response=%+v error=%v calls=%d", response, err, core.calls)
	}
}

// TEST-ISOLATION-010
func TestLifecycleEventsExposeCorrelationWithoutArguments(t *testing.T) {
	store, _ := NewStore(testCatalog(t))
	recorder := &eventRecorder{}
	client := &fakeClient{response: &contract.ExecutionResponse{Result: contract.ResultSuccess}}
	gateway, err := NewGatewayWithSink(store, map[config.RunnerProfile]Client{config.ProfileDiagnostics: client}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := contract.NewExecutionRequest("system_info", map[string]string{"secret": "must-not-be-recorded"}, contract.Metadata{})
	if _, err := gateway.Dispatch(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	handler, err := NewProfileHandlerWithSink(config.ProfileDiagnostics, store, &fakeCore{}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Handle(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 2 {
		t.Fatalf("events=%d", len(recorder.events))
	}
	for _, event := range recorder.events {
		if event.RequestID != request.RequestID || event.CatalogVersion != "1.0.0" || event.Profile != config.ProfileDiagnostics || event.Operation != "system_info" || event.Timestamp.IsZero() {
			t.Fatalf("incomplete event: %+v", event)
		}
	}
}

func TestConstructorsRejectInvalidInputs(t *testing.T) {
	if _, err := NewSnapshot(nil); err == nil {
		t.Fatal("nil snapshot accepted")
	}
	if _, err := NewStore(nil); err == nil {
		t.Fatal("nil store accepted")
	}
	if err := (*Store)(nil).Activate(testCatalog(t)); err == nil {
		t.Fatal("nil store activation accepted")
	}
	store, _ := NewStore(testCatalog(t))
	if _, err := NewGateway(nil, nil); err == nil {
		t.Fatal("nil gateway catalog accepted")
	}
	if _, err := NewGateway(store, map[config.RunnerProfile]Client{config.ProfileDiagnostics: nil}); err == nil {
		t.Fatal("nil client accepted")
	}
	if _, err := NewProfileHandler("ghost", store, &fakeCore{}); err == nil {
		t.Fatal("unknown profile accepted")
	}
}
