package agent

import (
	"context"
	"errors"
	"testing"

	"safeterm/internal/contract"
)

// ---- test doubles -----------------------------------------------------
//
// Each mock records whether it was called, so tests can assert that a
// failure at an earlier stage prevents every later stage from running at
// all (REQ-002, AC-1..AC-4) — not just that the final Result is "rejected".

type mockValidator struct {
	called  bool
	failure error
}

func (m *mockValidator) Validate(_ *contract.ExecutionRequest) error {
	m.called = true
	return m.failure
}

type mockPolicy struct {
	called  bool
	failure error
}

func (m *mockPolicy) Authorize(_ *contract.ExecutionRequest) error {
	m.called = true
	return m.failure
}

type mockTranslator struct {
	called  bool
	failure error
}

func (m *mockTranslator) Translate(req *contract.ExecutionRequest) (string, []string, error) {
	m.called = true
	if m.failure != nil {
		return "", nil, m.failure
	}
	return req.Command, nil, nil
}

type mockExecutor struct {
	called   bool
	failure  error
	exitCode int
}

func (m *mockExecutor) Execute(_ context.Context, command string, _ []string) (string, string, int, error) {
	m.called = true
	if m.failure != nil {
		return "", "boom", m.exitCode, m.failure
	}
	return "ran: " + command, "", 0, nil
}

func newTestRequest(t *testing.T) *contract.ExecutionRequest {
	t.Helper()
	req, err := contract.NewExecutionRequest("system_info", nil, contract.Metadata{})
	if err != nil {
		t.Fatalf("failed to build test request: %v", err)
	}
	return req
}

// ---- tests --------------------------------------------------------------

// TEST-AGENT-001
// Requirement: REQ-002 (AC-5)
// Scenario: a request that passes every stage results in ResultSuccess,
// with the RequestID preserved and every stage invoked exactly once.
func TestHandleRequest_AllStagesPass_ReturnsSuccess(t *testing.T) {
	v := &mockValidator{}
	p := &mockPolicy{}
	tr := &mockTranslator{}
	ex := &mockExecutor{}
	core := NewCore(v, p, tr, ex)

	req := newTestRequest(t)
	resp := core.HandleRequest(context.Background(), req)

	if resp.Result != contract.ResultSuccess {
		t.Fatalf("expected ResultSuccess, got %s (message: %s)", resp.Result, resp.Message)
	}
	if resp.RequestID != req.RequestID {
		t.Fatalf("RequestID mismatch: got %s want %s", resp.RequestID, req.RequestID)
	}
	if !v.called || !p.called || !tr.called || !ex.called {
		t.Fatal("expected all four stages to be called when every stage passes")
	}
}

// TEST-AGENT-002
// Requirement: REQ-002 (AC-1)
// Scenario: if Validation fails, Policy/Translation/Execution must never
// be called — this is the core "stop at first failure" guarantee.
func TestHandleRequest_ValidationFails_StopsPipelineImmediately(t *testing.T) {
	v := &mockValidator{failure: errors.New("bad argument")}
	p := &mockPolicy{}
	tr := &mockTranslator{}
	ex := &mockExecutor{}
	core := NewCore(v, p, tr, ex)

	resp := core.HandleRequest(context.Background(), newTestRequest(t))

	if resp.Result != contract.ResultRejected {
		t.Fatalf("expected ResultRejected, got %s", resp.Result)
	}
	if p.called || tr.called || ex.called {
		t.Fatal("expected Policy/Translator/Executor to be skipped after Validation failure")
	}
}

// TEST-AGENT-003
// Requirement: REQ-002 (AC-2)
// Scenario: if Policy rejects the request, Translation/Execution must
// never run, even though Validation already passed.
func TestHandleRequest_PolicyFails_StopsBeforeTranslation(t *testing.T) {
	v := &mockValidator{}
	p := &mockPolicy{failure: errors.New("role not permitted")}
	tr := &mockTranslator{}
	ex := &mockExecutor{}
	core := NewCore(v, p, tr, ex)

	resp := core.HandleRequest(context.Background(), newTestRequest(t))

	if resp.Result != contract.ResultRejected {
		t.Fatalf("expected ResultRejected, got %s", resp.Result)
	}
	if !v.called {
		t.Fatal("expected Validator to have been called")
	}
	if tr.called || ex.called {
		t.Fatal("expected Translator/Executor to be skipped after Policy failure")
	}
}

// TEST-AGENT-004
// Requirement: REQ-002 (AC-3)
// Scenario: if Translation fails, Execution must never run — the system
// must never attempt to execute an untranslated / unmapped command.
func TestHandleRequest_TranslationFails_StopsBeforeExecution(t *testing.T) {
	v := &mockValidator{}
	p := &mockPolicy{}
	tr := &mockTranslator{failure: errors.New("no mapping for operation")}
	ex := &mockExecutor{}
	core := NewCore(v, p, tr, ex)

	resp := core.HandleRequest(context.Background(), newTestRequest(t))

	if resp.Result != contract.ResultRejected {
		t.Fatalf("expected ResultRejected, got %s", resp.Result)
	}
	if ex.called {
		t.Fatal("expected Executor to be skipped after Translation failure")
	}
}

// TEST-AGENT-005
// Requirement: REQ-002 (AC-4)
// Scenario: if Execution itself fails (the command ran but errored),
// the result must be ResultExecutionError, not ResultRejected — this
// distinction matters for audit (M8): "rejected" means SafeTerm refused
// to run it; "execution_error" means it ran and the underlying tool failed.
func TestHandleRequest_ExecutionFails_ReturnsExecutionError(t *testing.T) {
	v := &mockValidator{}
	p := &mockPolicy{}
	tr := &mockTranslator{}
	ex := &mockExecutor{failure: errors.New("command not found"), exitCode: 127}
	core := NewCore(v, p, tr, ex)

	resp := core.HandleRequest(context.Background(), newTestRequest(t))

	if resp.Result != contract.ResultExecutionError {
		t.Fatalf("expected ResultExecutionError, got %s", resp.Result)
	}
	if resp.ExitCode != 127 {
		t.Fatalf("expected exit code 127 to be preserved, got %d", resp.ExitCode)
	}
}

// TEST-AGENT-006
// Requirement: REQ-002 (AC-6)
// Scenario: StartedAt/EndedAt must always be populated, on both success
// and rejection paths, since Audit (M8) will need duration for every
// outcome, not just successful ones.
func TestHandleRequest_AlwaysPopulatesTimestamps(t *testing.T) {
	v := &mockValidator{failure: errors.New("reject early")}
	core := NewCore(v, &mockPolicy{}, &mockTranslator{}, &mockExecutor{})

	resp := core.HandleRequest(context.Background(), newTestRequest(t))

	if resp.StartedAt.IsZero() || resp.EndedAt.IsZero() {
		t.Fatal("expected StartedAt and EndedAt to be populated even on rejection")
	}
}

// TEST-AGENT-007
// Scenario: the M2 placeholder stages (PassThroughValidator, AllowAllPolicy,
// IdentityTranslator, NoOpExecutor) must wire together into a working Core,
// proving the pipeline itself is correct independent of any real business
// logic. This test exists specifically to justify placeholders.go — if this
// ever fails, the M2 vertical slice is broken.
func TestHandleRequest_WithM2Placeholders_EndToEndSucceeds(t *testing.T) {
	core := NewCore(PassThroughValidator{}, AllowAllPolicy{}, IdentityTranslator{}, NoOpExecutor{})

	resp := core.HandleRequest(context.Background(), newTestRequest(t))

	if resp.Result != contract.ResultSuccess {
		t.Fatalf("expected placeholder pipeline to succeed end-to-end, got %s (%s)", resp.Result, resp.Message)
	}
}
