//go:build linux

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/isolation"
)

type isolationCore struct {
	calls   int
	profile string
}

func (c *isolationCore) HandleRequest(_ context.Context, request *contract.ExecutionRequest) *contract.ExecutionResponse {
	c.calls++
	return &contract.ExecutionResponse{RequestID: request.RequestID, Result: contract.ResultSuccess, Stdout: c.profile}
}

// TEST-ISOLATION-004, TEST-ISOLATION-006, TEST-ISOLATION-010, TEST-ISOLATION-011
func TestCapabilityIsolatedGatewayRoutesAcrossAuthenticatedUnixIPC(t *testing.T) {
	cfg, err := config.LoadConfig("../../internal/defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	store, err := isolation.NewStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	uid := uint32(os.Getuid())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type runningProfile struct {
		core   *isolationCore
		client *isolation.UnixClient
	}
	running := map[config.RunnerProfile]runningProfile{}
	for _, profile := range []config.RunnerProfile{config.ProfileDiagnostics, config.ProfileNetworkObserver} {
		core := &isolationCore{profile: string(profile)}
		handler, err := isolation.NewProfileHandler(profile, store, core)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), string(profile)+".sock")
		server, err := isolation.ListenUnix(path, 0o600, []uint32{uid}, handler)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { server.Close() })
		go server.Serve(ctx)
		client, err := isolation.NewUnixClient(path, []uint32{uid})
		if err != nil {
			t.Fatal(err)
		}
		running[profile] = runningProfile{core: core, client: client}
	}
	gateway, err := isolation.NewGateway(store, map[config.RunnerProfile]isolation.Client{
		config.ProfileDiagnostics:     running[config.ProfileDiagnostics].client,
		config.ProfileNetworkObserver: running[config.ProfileNetworkObserver].client,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		operation string
		profile   config.RunnerProfile
	}{
		{"system_info", config.ProfileDiagnostics}, {"capture", config.ProfileNetworkObserver},
	} {
		request, err := contract.NewExecutionRequest(tc.operation, nil, contract.Metadata{})
		if err != nil {
			t.Fatal(err)
		}
		response, err := gateway.Dispatch(context.Background(), request)
		if err != nil || response.RequestID != request.RequestID || response.Stdout != string(tc.profile) {
			t.Fatalf("%s response=%+v error=%v", tc.operation, response, err)
		}
	}
	if running[config.ProfileDiagnostics].core.calls != 1 || running[config.ProfileNetworkObserver].core.calls != 1 {
		t.Fatalf("unexpected core calls: %+v", running)
	}

	crossProfile, _ := contract.NewExecutionRequest("capture", nil, contract.Metadata{})
	response, err := running[config.ProfileDiagnostics].client.Handle(context.Background(), crossProfile)
	if err != nil || response.Result != contract.ResultRejected {
		t.Fatalf("cross-profile response=%+v error=%v", response, err)
	}
	if running[config.ProfileDiagnostics].core.calls != 1 {
		t.Fatal("cross-profile request reached diagnostics core")
	}
}
