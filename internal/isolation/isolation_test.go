package isolation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"safeterm/internal/audit"
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

// TEST-ISOLATION-002, TEST-ISOLATION-004, TEST-INTEGRITY-017
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
	if len(filtered.Profiles) != 1 || len(filtered.Operations) < 3 || len(filtered.Executables) < 3 {
		t.Fatalf("unexpected diagnostics snapshot: %+v", filtered)
	}
	filtered.Operations[0].Name = "mutated"
	second, _ := snapshot.ProfileConfig(config.ProfileDiagnostics)
	if second.Operations[0].Name == "mutated" {
		t.Fatal("profile config aliased active snapshot")
	}
	serviceManager, err := snapshot.ProfileConfig(config.ProfileServiceManager)
	if err != nil {
		t.Fatal(err)
	}
	serviceExecutables := map[string]bool{}
	for _, executable := range serviceManager.Executables {
		serviceExecutables[executable.ID] = true
	}
	if len(serviceManager.Executables) != 2 || !serviceExecutables["sudo"] || !serviceExecutables["systemctl"] {
		t.Fatalf("service-manager controller binding missing: %+v", serviceManager.Executables)
	}
	logOperation, ok := snapshot.Operation("log")
	if !ok {
		t.Fatal("log operation missing")
	}
	logOperation.Parameters[0].ValueMap["service_A"] = "/etc/shadow"
	secondLogOperation, _ := snapshot.Operation("log")
	if secondLogOperation.Parameters[0].ValueMap["service_A"] == "/etc/shadow" {
		t.Fatal("operation value map aliased active snapshot")
	}
	profileDefinition, _ := snapshot.Profile(config.ProfileServiceManager)
	profileDefinition.ControllerArgsPrefix[0] = "--changed"
	secondProfileDefinition, _ := snapshot.Profile(config.ProfileServiceManager)
	if secondProfileDefinition.ControllerArgsPrefix[0] == "--changed" {
		t.Fatal("profile controller prefix aliased active snapshot")
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
	if store.Current() != previous || store.Current().Version() != "1.2.0" {
		t.Fatal("failed activation changed current snapshot")
	}
	next := testCatalog(t)
	next.Version = "1.3.0"
	if err := store.Activate(next); err != nil {
		t.Fatal(err)
	}
	if store.Current() == previous || store.Current().Version() != "1.3.0" {
		t.Fatal("valid snapshot was not atomically activated")
	}
}

// TEST-INTEGRITY-014
func TestVerifiedStoreRejectsUnsignedActivationAndRollsBack(t *testing.T) {
	trusted := map[string]bool{"1.2.0": true}
	verifier := func(candidate *config.Config) error {
		if candidate == nil || !trusted[candidate.Version] {
			return errors.New("invalid signature")
		}
		return nil
	}
	store, err := NewVerifiedStore(testCatalog(t), verifier)
	if err != nil {
		t.Fatal(err)
	}
	unsigned := testCatalog(t)
	unsigned.Version = "1.3.0"
	if err := store.Activate(unsigned); err == nil || store.Current().Version() != "1.2.0" {
		t.Fatalf("unsigned activation error=%v version=%q", err, store.Current().Version())
	}
	trusted["1.3.0"] = true
	if err := store.Activate(unsigned); err != nil || store.Current().Version() != "1.3.0" {
		t.Fatalf("signed activation error=%v version=%q", err, store.Current().Version())
	}
	if err := store.Rollback(); err != nil || store.Current().Version() != "1.2.0" {
		t.Fatalf("rollback error=%v version=%q", err, store.Current().Version())
	}
	if err := store.Rollback(); err == nil {
		t.Fatal("rollback without history succeeded")
	}
	if _, err := NewVerifiedStore(testCatalog(t), nil); err == nil {
		t.Fatal("nil verifier accepted")
	}
	if err := (*Store)(nil).Rollback(); err == nil {
		t.Fatal("nil store rollback accepted")
	}
}

type fakeClient struct {
	calls    int
	response *contract.ExecutionResponse
}

type eventRecorder struct{ events []LifecycleEvent }

func (r *eventRecorder) Record(_ context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}

type failingEventSink struct{}

func (failingEventSink) Record(context.Context, audit.Event) error { return errors.New("offline") }

type failOnCallSink struct{ calls int }

func (s *failOnCallSink) Record(context.Context, audit.Event) error {
	s.calls++
	if s.calls == 2 {
		return errors.New("completion offline")
	}
	return nil
}

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
	if len(recorder.events) != 4 {
		t.Fatalf("events=%d", len(recorder.events))
	}
	for _, event := range recorder.events {
		if event.RequestID != request.RequestID || event.CatalogVersion != "1.2.0" || event.Profile != string(config.ProfileDiagnostics) || event.Operation != "system_info" || event.OccurredAt.IsZero() {
			t.Fatalf("incomplete event: %+v", event)
		}
	}
	if len(recorder.events[0].ArgumentNames) != 1 || recorder.events[0].ArgumentNames[0] != "secret" {
		t.Fatalf("argument names not sanitized: %+v", recorder.events[0])
	}
}

// TEST-AUDIT-006
func TestAuditFailureStopsBeforeRoutingOrExecution(t *testing.T) {
	store, _ := NewStore(testCatalog(t))
	client := &fakeClient{response: &contract.ExecutionResponse{Result: contract.ResultSuccess}}
	gateway, err := NewGatewayWithSink(store, map[config.RunnerProfile]Client{config.ProfileDiagnostics: client}, failingEventSink{})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := contract.NewExecutionRequest("system_info", nil, contract.Metadata{})
	if _, err := gateway.Dispatch(context.Background(), request); err == nil || client.calls != 0 {
		t.Fatalf("gateway audit failure did not fail closed: err=%v calls=%d", err, client.calls)
	}
	core := &fakeCore{}
	handler, err := NewProfileHandlerWithSink(config.ProfileDiagnostics, store, core, failingEventSink{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Handle(context.Background(), request); err == nil || core.calls != 0 {
		t.Fatalf("runner audit failure did not fail closed: err=%v calls=%d", err, core.calls)
	}
	completionSink := &failOnCallSink{}
	completionCore := &fakeCore{}
	completionHandler, err := NewProfileHandlerWithSink(config.ProfileDiagnostics, store, completionCore, completionSink)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := completionHandler.Handle(context.Background(), request); err == nil || completionCore.calls != 1 {
		t.Fatalf("completion audit failure was hidden: err=%v calls=%d", err, completionCore.calls)
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
