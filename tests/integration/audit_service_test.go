//go:build linux

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"safeterm/internal/audit"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/isolation"
)

type auditedCore struct{ calls int }

func (c *auditedCore) HandleRequest(_ context.Context, request *contract.ExecutionRequest) *contract.ExecutionResponse {
	c.calls++
	start := time.Now().UTC()
	return &contract.ExecutionResponse{RequestID: request.RequestID, Result: contract.ResultSuccess, ExitCode: 0, StartedAt: start, EndedAt: start.Add(time.Millisecond)}
}

// TEST-AUDIT-001..007
func TestGatewayAndRunnerPersistCorrelatedAuditTrail(t *testing.T) {
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := isolation.NewStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	storePath := filepath.Join(directory, "audit.jsonl")
	store, err := audit.OpenFileStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	uid := uint32(os.Getuid())
	auditSocket := filepath.Join(directory, "audit.sock")
	auditServer, err := audit.ListenUnix(auditSocket, 0o600, map[uint32][]audit.Producer{uid: {
		{Component: audit.ComponentGateway},
		{Component: audit.ComponentRunner, Profile: string(config.ProfileDiagnostics)},
	}}, store)
	if err != nil {
		t.Fatal(err)
	}
	defer auditServer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go auditServer.Serve(ctx)
	auditClient, err := audit.NewUnixClient(auditSocket, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}

	core := &auditedCore{}
	handler, err := isolation.NewProfileHandlerWithSink(config.ProfileDiagnostics, catalog, core, auditClient)
	if err != nil {
		t.Fatal(err)
	}
	runnerSocket := filepath.Join(directory, "runner.sock")
	runnerServer, err := isolation.ListenUnix(runnerSocket, 0o600, []uint32{uid}, handler)
	if err != nil {
		t.Fatal(err)
	}
	defer runnerServer.Close()
	go runnerServer.Serve(ctx)
	runnerClient, err := isolation.NewUnixClient(runnerSocket, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := isolation.NewGatewayWithSink(catalog, map[config.RunnerProfile]isolation.Client{
		config.ProfileDiagnostics: runnerClient,
	}, auditClient)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := contract.NewExecutionRequest("system_info", map[string]string{"credential": "never-log-this"}, contract.Metadata{
		BrokerType: "cli", UserID: "alice", ClaimedRoles: []string{"viewer"},
	})
	response, err := gateway.Dispatch(context.Background(), request)
	if err != nil || response.Result != contract.ResultSuccess || core.calls != 1 {
		t.Fatalf("response=%+v error=%v calls=%d", response, err, core.calls)
	}
	entries, err := audit.ReadFile(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("audit entries=%d, want 4", len(entries))
	}
	wantOutcomes := []string{"routed", "accepted", "completed", "returned"}
	for i, entry := range entries {
		if entry.Event.RequestID != request.RequestID || entry.Event.CatalogVersion != "1.1.0" || entry.Event.Outcome != wantOutcomes[i] {
			t.Fatalf("entry %d = %+v", i, entry)
		}
	}
	data, _ := os.ReadFile(storePath)
	if strings.Contains(string(data), "never-log-this") {
		t.Fatal("raw argument value leaked into audit store")
	}
}
