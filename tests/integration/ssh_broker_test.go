package integration

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/isolation"
)

type authenticatedRunner struct {
	request *contract.ExecutionRequest
}

// TEST-BROKER-011 proves the deployed Broker is an unprivileged network
// frontend with no execution capabilities and only the required IPC group.
func TestSSHBrokerHasHardenedSystemdUnit(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "systemd", "safeterm-ssh-broker.service"))
	if err != nil {
		t.Fatal(err)
	}
	unit := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, required := range []string{
		"User=safeterm-broker", "Group=safeterm-broker", "SupplementaryGroups=safeterm-gateway",
		"ExecStart=/usr/local/bin/safeterm-ssh-broker", "NoNewPrivileges=true",
		"CapabilityBoundingSet=\nAmbientCapabilities=\n", "ProtectSystem=strict",
		"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6", "MemoryDenyWriteExecute=true",
		"MemoryMax=128M", "TasksMax=64", "LimitNOFILE=512",
	} {
		if !strings.Contains(unit, required) {
			t.Errorf("SSH Broker unit missing %q", required)
		}
	}
	if strings.Contains(unit, "safeterm-runner") || strings.Contains(unit, "CAP_NET_RAW") || strings.Contains(unit, "CAP_SYS_ADMIN") {
		t.Fatal("SSH Broker unit has execution or elevated capability references")
	}
}

func (r *authenticatedRunner) Handle(_ context.Context, request *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	r.request = request
	return &contract.ExecutionResponse{RequestID: request.RequestID, Result: contract.ResultSuccess, Stdout: "safe host\n"}, nil
}

// TEST-BROKER-010 proves authenticated SSH identity and roles cross the
// Broker -> Gateway route while the user supplies only an operation name.
func TestRestrictedShellRoutesAuthenticatedIdentityThroughGateway(t *testing.T) {
	cfg := &config.Config{
		Version:     "m12-integration",
		Profiles:    []config.ProfileDefinition{{Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/diagnostics.sock", RunAsUser: "diagnostics", MemoryLimitMiB: 64, ProcessLimit: 8}},
		Executables: []config.ExecutableDefinition{{ID: "uname", Path: "/usr/bin/uname", SHA256: strings.Repeat("a", 64), AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics}}},
		Operations:  []config.OperationDefinition{{Name: "system_info", Description: "System information", RunnerProfile: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, ExecutableRef: "uname", ArgsTemplate: []string{"-a"}}},
		Roles:       []config.RoleDefinition{{Name: "viewer"}},
		Policies:    []config.PolicyDefinition{{Role: "viewer", Operations: []string{"system_info"}}},
	}
	store, err := isolation.NewStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	runner := &authenticatedRunner{}
	gateway, err := isolation.NewGateway(store, map[config.RunnerProfile]isolation.Client{config.ProfileDiagnostics: runner})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := broker.NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	terminal := &bytes.Buffer{}
	shell, err := broker.NewShell(gateway, catalog, broker.Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader("run system_info\rexit\r"), terminal)
	if err != nil {
		t.Fatal(err)
	}
	if err := shell.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.request == nil || runner.request.Metadata.UserID != "alice" || runner.request.Metadata.BrokerType != "ssh" || strings.Join(runner.request.Metadata.ClaimedRoles, ",") != "viewer" {
		t.Fatalf("authenticated metadata was not routed: %+v", runner.request)
	}
	if !strings.Contains(terminal.String(), "safe host") || !strings.Contains(terminal.String(), "result: success") {
		t.Fatalf("unexpected terminal output: %q", terminal.String())
	}
}
