package integration

import (
	"context"
	"strings"
	"testing"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/policy"
	"safeterm/internal/validation"
)

// These doubles record reachability without translating or running OS commands.
type policyTestTranslator struct{ calls int }

func (tr *policyTestTranslator) Translate(req *contract.ExecutionRequest) (string, []string, error) {
	tr.calls++
	return req.Command, nil, nil
}

type policyTestExecutor struct{ calls int }

func (ex *policyTestExecutor) Execute(context.Context, string, []string) (string, string, int, error) {
	ex.calls++
	return "policy test output", "", 0, nil
}

// The wrapper delegates every decision to the real policy and records its calls.
type observedPolicy struct {
	delegate agent.PolicyEnforcer
	calls    int
}

func (p *observedPolicy) Authorize(req *contract.ExecutionRequest) error {
	p.calls++
	return p.delegate.Authorize(req)
}

func buildCoreWithRealPolicy(t *testing.T) (*agent.Core, *observedPolicy, *policyTestTranslator, *policyTestExecutor) {
	t.Helper()
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatal(err)
	}
	p := &observedPolicy{delegate: policy.New(cfg)}
	tr := &policyTestTranslator{}
	ex := &policyTestExecutor{}
	return agent.NewCore(validation.New(cfg), p, tr, ex), p, tr, ex
}

func policyRequest(t *testing.T, operation string, roles []string) *contract.ExecutionRequest {
	t.Helper()
	var args map[string]string
	if operation == "trace" {
		args = map[string]string{"port": "999", "host": "192.168.10.20"}
	}
	req, err := contract.NewExecutionRequest(operation, args, contract.Metadata{ClaimedRoles: roles})
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func assertPolicyResponse(t *testing.T, req *contract.ExecutionRequest, resp *contract.ExecutionResponse) {
	t.Helper()
	if resp.RequestID != req.RequestID || resp.StartedAt.IsZero() || resp.EndedAt.IsZero() || resp.EndedAt.Before(resp.StartedAt) {
		t.Fatalf("invalid response correlation or timestamps: %+v", resp)
	}
}

// TEST-INTEG-004
// Requirement: REQ-005 (AC-2, AC-3, AC-9), REQ-002 (AC-5, AC-6)
// Real configuration, validation and authorization allow the documented grants.
func TestCore_WithRealPolicy_AcceptsAuthorizedRequests(t *testing.T) {
	for _, tc := range []struct {
		name, operation string
		roles           []string
	}{
		{"viewer system info", "system_info", []string{"viewer"}},
		{"netops system info", "system_info", []string{"netops"}},
		{"netops trace", "trace", []string{"netops"}},
		{"multiple roles", "trace", []string{"ghost", "viewer", "netops"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, p, tr, ex := buildCoreWithRealPolicy(t)
			req := policyRequest(t, tc.operation, tc.roles)
			resp := core.HandleRequest(context.Background(), req)
			assertPolicyResponse(t, req, resp)
			if resp.Result != contract.ResultSuccess || resp.Stdout != "policy test output" || resp.ExitCode != 0 {
				t.Fatalf("expected successful test execution, got %+v", resp)
			}
			if p.calls != 1 || tr.calls != 1 || ex.calls != 1 {
				t.Fatalf("unexpected stage calls: policy=%d translation=%d execution=%d", p.calls, tr.calls, ex.calls)
			}
		})
	}
}

// TEST-INTEG-005
// Requirement: REQ-005 (AC-1..AC-4, AC-9), REQ-002 (AC-2, AC-6)
// Rejected roles must never reach translation or execution.
func TestCore_WithRealPolicy_DenialStopsDownstreamStages(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roles []string
	}{
		{"missing", nil}, {"empty", []string{}}, {"unknown", []string{"ghost"}},
		{"viewer", []string{"viewer"}}, {"none permitted", []string{"viewer", "ghost"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, p, tr, ex := buildCoreWithRealPolicy(t)
			req := policyRequest(t, "trace", tc.roles)
			resp := core.HandleRequest(context.Background(), req)
			assertPolicyResponse(t, req, resp)
			if resp.Result != contract.ResultRejected || !strings.HasPrefix(resp.Message, "policy: ") {
				t.Fatalf("expected policy rejection, got %+v", resp)
			}
			if p.calls != 1 || tr.calls != 0 || ex.calls != 0 {
				t.Fatalf("policy denial failed to stop pipeline: policy=%d translation=%d execution=%d", p.calls, tr.calls, ex.calls)
			}
		})
	}
}

// TEST-INTEG-006
// Requirement: REQ-005 (AC-9), REQ-002 (AC-1), REQ-004 (AC-1, AC-3, AC-4, AC-6)
// An authorized role cannot bypass validation or submit raw commands.
func TestCore_WithRealPolicy_InvalidInputStopsBeforePolicy(t *testing.T) {
	for _, name := range []string{"unknown operation", "raw command", "port range", "injection", "extra argument"} {
		t.Run(name, func(t *testing.T) {
			core, p, tr, ex := buildCoreWithRealPolicy(t)
			req := policyRequest(t, "trace", []string{"netops"})
			switch name {
			case "unknown operation":
				req.Command = "missing"
			case "raw command":
				req.Command = "tcpdump"
			case "port range":
				req.Arguments["port"] = "65536"
			case "injection":
				req.Arguments["port"] = "; rm -rf /"
			case "extra argument":
				req.Arguments["extra"] = "unexpected"
			}
			resp := core.HandleRequest(context.Background(), req)
			assertPolicyResponse(t, req, resp)
			if resp.Result != contract.ResultRejected || !strings.HasPrefix(resp.Message, "validation: ") {
				t.Fatalf("expected validation rejection, got %+v", resp)
			}
			if p.calls != 0 || tr.calls != 0 || ex.calls != 0 {
				t.Fatalf("validation failed to stop pipeline: policy=%d translation=%d execution=%d", p.calls, tr.calls, ex.calls)
			}
		})
	}
}
