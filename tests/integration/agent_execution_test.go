package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/execution"
	"safeterm/internal/policy"
	"safeterm/internal/translation"
	"safeterm/internal/validation"
)

const executionIntegrationEnvironment = "SAFETERM_EXECUTION_INTEGRATION_HELPER"

func integrationHelperStderr(stderr string) string {
	// This package contains only external integration tests, so Go cannot
	// instrument its helper subprocess. During `go test -cover`, the coverage
	// runtime appends this diagnostic after the helper exits.
	return strings.TrimSuffix(stderr, "program not built with -cover\n")
}

func TestExecutionIntegrationHelper(t *testing.T) {
	if os.Getenv(executionIntegrationEnvironment) != "1" {
		return
	}
	separator := -1
	for i, value := range os.Args {
		if value == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(97)
	}
	switch os.Args[separator+1] {
	case "success":
		fmt.Fprint(os.Stdout, "integration-stdout")
		fmt.Fprint(os.Stderr, "integration-stderr")
		os.Exit(0)
	case "exit":
		fmt.Fprint(os.Stdout, "integration-stdout-before-exit")
		fmt.Fprint(os.Stderr, "integration-stderr-before-exit")
		os.Exit(37)
	case "wait":
		fmt.Fprint(os.Stdout, "integration-stdout-before-wait")
		fmt.Fprint(os.Stderr, "integration-stderr-before-wait")
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		os.Exit(96)
	}
}

type observedExecutor struct {
	delegate agent.Executor
	calls    int
}

func (ex *observedExecutor) Execute(ctx context.Context, command string, args []string) (string, string, int, error) {
	ex.calls++
	return ex.delegate.Execute(ctx, command, args)
}

func executionIntegrationConfig() *config.Config {
	operations := make([]config.OperationDefinition, 0, 3)
	for _, action := range []string{"success", "exit", "wait"} {
		operations = append(operations, config.OperationDefinition{
			Name:          "probe_" + action,
			RunnerProfile: config.ProfileDiagnostics,
			ExecutorKind:  config.ExecutorProcess,
			ExecutableRef: "test-helper",
			ArgsTemplate:  []string{"-test.run=^TestExecutionIntegrationHelper$", "--", action},
		})
	}
	return &config.Config{
		Version:     "test-v1",
		Profiles:    []config.ProfileDefinition{{Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/safeterm/test.sock", RunAsUser: "test", MemoryLimitMiB: 64, ProcessLimit: 8}},
		Executables: []config.ExecutableDefinition{{ID: "test-helper", Path: os.Args[0], SHA256: "0000000000000000000000000000000000000000000000000000000000000000", AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics}}},
		Operations:  operations,
		Roles:       []config.RoleDefinition{{Name: "operator"}, {Name: "viewer"}},
		Policies: []config.PolicyDefinition{
			{Role: "operator", Operations: []string{"probe_success", "probe_exit", "probe_wait"}},
			{Role: "viewer", Operations: []string{}},
		},
	}
}

func buildCoreWithRealExecutor(t *testing.T, timeout time.Duration) (*agent.Core, *observedExecutor) {
	t.Helper()
	t.Setenv(executionIntegrationEnvironment, "1")
	cfg := executionIntegrationConfig()
	if err := config.ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	translator, err := translation.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := execution.New(timeout)
	if err != nil {
		t.Fatal(err)
	}
	observed := &observedExecutor{delegate: runner}
	return agent.NewCore(validation.New(cfg), policy.New(cfg), translator, observed), observed
}

func executionRequest(t *testing.T, operation string, roles []string, args map[string]string) *contract.ExecutionRequest {
	t.Helper()
	req, err := contract.NewExecutionRequest(operation, args, contract.Metadata{ClaimedRoles: roles})
	if err != nil {
		t.Fatal(err)
	}
	return req
}

// TEST-INTEG-010
// Requirement: REQ-007 (AC-8)
func TestCore_WithRealExecutor_RunsConfiguredProcess(t *testing.T) {
	core, executor := buildCoreWithRealExecutor(t, 10*time.Second)
	req := executionRequest(t, "probe_success", []string{"operator"}, nil)
	resp := core.HandleRequest(context.Background(), req)
	assertPolicyResponse(t, req, resp)
	if resp.Result != contract.ResultSuccess || resp.Stdout != "integration-stdout" || integrationHelperStderr(resp.Stderr) != "integration-stderr" || resp.ExitCode != 0 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

// TEST-INTEG-011
// Requirement: REQ-007 (AC-2, AC-10), REQ-002 (AC-4)
func TestCore_WithRealExecutor_NonZeroExitIsExecutionError(t *testing.T) {
	core, executor := buildCoreWithRealExecutor(t, 10*time.Second)
	req := executionRequest(t, "probe_exit", []string{"operator"}, nil)
	resp := core.HandleRequest(context.Background(), req)
	assertPolicyResponse(t, req, resp)
	if resp.Result != contract.ResultExecutionError || resp.ExitCode != 37 ||
		resp.Stdout != "integration-stdout-before-exit" || integrationHelperStderr(resp.Stderr) != "integration-stderr-before-exit" ||
		!strings.HasPrefix(resp.Message, "execution: ") {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

// TEST-INTEG-012
// Requirement: REQ-007 (AC-4, AC-10), REQ-002 (AC-4)
func TestCore_WithRealExecutor_TimeoutIsExecutionError(t *testing.T) {
	core, executor := buildCoreWithRealExecutor(t, 100*time.Millisecond)
	req := executionRequest(t, "probe_wait", []string{"operator"}, nil)
	resp := core.HandleRequest(context.Background(), req)
	assertPolicyResponse(t, req, resp)
	if resp.Result != contract.ResultExecutionError || resp.ExitCode != execution.ExitCodeUnavailable ||
		resp.Stdout != "integration-stdout-before-wait" || resp.Stderr != "integration-stderr-before-wait" ||
		!strings.Contains(resp.Message, context.DeadlineExceeded.Error()) {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

// TEST-INTEG-013
// Requirement: REQ-007 (AC-9), REQ-004 (AC-1, AC-4, AC-6), REQ-005 (AC-1, AC-4)
func TestCore_WithRealExecutor_UntrustedRequestsNeverReachExecution(t *testing.T) {
	for _, name := range []string{"unknown operation", "raw executable", "unexpected argument", "injection argument", "missing roles", "unknown role", "unauthorized role"} {
		t.Run(name, func(t *testing.T) {
			core, executor := buildCoreWithRealExecutor(t, 10*time.Second)
			req := executionRequest(t, "probe_success", []string{"operator"}, nil)
			stage := "validation: "
			switch name {
			case "unknown operation":
				req.Command = "missing"
			case "raw executable":
				req.Command = os.Args[0]
			case "unexpected argument":
				req.Arguments["extra"] = "value"
			case "injection argument":
				req.Arguments["extra"] = "; echo injected"
			case "missing roles":
				req.Metadata.ClaimedRoles, stage = nil, "policy: "
			case "unknown role":
				req.Metadata.ClaimedRoles, stage = []string{"ghost"}, "policy: "
			case "unauthorized role":
				req.Metadata.ClaimedRoles, stage = []string{"viewer"}, "policy: "
			}
			resp := core.HandleRequest(context.Background(), req)
			assertPolicyResponse(t, req, resp)
			if resp.Result != contract.ResultRejected || !strings.HasPrefix(resp.Message, stage) {
				t.Fatalf("expected %s rejection, got %+v", stage, resp)
			}
			if executor.calls != 0 {
				t.Fatalf("real executor was called %d times", executor.calls)
			}
		})
	}
}
