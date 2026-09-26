// Package integration contains cross-package tests that wire real
// implementations from multiple milestones together, as opposed to the
// unit tests inside each package which test one component in isolation.
package integration

import (
	"context"
	"testing"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/validation"
)

// buildCoreWithRealValidator loads the actual shipped repository config
// and wires the real Validator (M4) into Agent Core (M2), while Policy,
// Translation and Execution remain the M2 placeholders (still correct,
// since M5/M6/M7 do not exist yet).
func buildCoreWithRealValidator(t *testing.T) *agent.Core {
	t.Helper()
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatalf("failed to load shipped config: %v", err)
	}
	v := validation.New(cfg)
	return agent.NewCore(v, agent.AllowAllPolicy{}, agent.IdentityTranslator{}, agent.NoOpExecutor{})
}

// TEST-INTEG-001
// Requirement: REQ-002 + REQ-004
// Scenario: this is the exact walk-through from the project document
// (section 9 — "نمونه جریان اجرای دستور"): a user submits
// trace 192.168.10.20:999, and the request must be accepted by Agent
// Core once a real Validator is wired in, proving Core correctly calls
// whichever Validator implementation it is given.
func TestCore_WithRealValidator_AcceptsValidTraceRequest(t *testing.T) {
	core := buildCoreWithRealValidator(t)

	req, err := contract.NewExecutionRequest("trace", map[string]string{
		"port": "999",
		"host": "192.168.10.20",
	}, contract.Metadata{})
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	resp := core.HandleRequest(context.Background(), req)

	if resp.Result != contract.ResultSuccess {
		t.Fatalf("expected ResultSuccess, got %s (message: %s)", resp.Result, resp.Message)
	}
}

// TEST-INTEG-002
// Scenario: an out-of-range port must be rejected by Core end-to-end —
// i.e. the rejection actually comes from Validation, not from some
// downstream stage silently failing.
func TestCore_WithRealValidator_RejectsOutOfRangePort(t *testing.T) {
	core := buildCoreWithRealValidator(t)

	req, err := contract.NewExecutionRequest("trace", map[string]string{
		"port": "99999",
		"host": "192.168.10.20",
	}, contract.Metadata{})
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	resp := core.HandleRequest(context.Background(), req)

	if resp.Result != contract.ResultRejected {
		t.Fatalf("expected ResultRejected for out-of-range port, got %s (%s)", resp.Result, resp.Message)
	}
}

// TEST-INTEG-003
// Scenario: the project document's own command-injection example
// (port = "; rm -rf /") must be rejected end-to-end through the real
// Agent Core pipeline, not just at the unit-test level.
func TestCore_WithRealValidator_RejectsInjectionAttempt(t *testing.T) {
	core := buildCoreWithRealValidator(t)

	req, err := contract.NewExecutionRequest("trace", map[string]string{
		"port": "; rm -rf /",
		"host": "192.168.10.20",
	}, contract.Metadata{})
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	resp := core.HandleRequest(context.Background(), req)

	if resp.Result != contract.ResultRejected {
		t.Fatalf("expected ResultRejected for injection attempt, got %s (%s)", resp.Result, resp.Message)
	}
}
