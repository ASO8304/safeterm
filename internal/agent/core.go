// Package agent implements Agent Core: the single, centralized point where
// every ExecutionRequest is validated, authorized, translated and executed
// (see docs/requirements/REQ-002 and docs/decisions/ADR-003).
//
// Agent Core itself does not know HOW to validate, authorize, translate or
// execute a request — it only knows the FIXED ORDER those steps must happen
// in, and that it must stop immediately at the first failure. The actual
// logic for each step is injected as an interface implementation ("Stage"),
// so each future milestone (M4 Validation, M5 Policy, M6 Translation,
// M7 Execution) can be built and tested independently, then plugged in here
// without changing Core itself.
package agent

import (
	"context"
	"time"

	"safeterm/internal/contract"
)

// Validator checks that a request's arguments are well-formed.
// The real implementation is in internal/validation (M4).
type Validator interface {
	Validate(req *contract.ExecutionRequest) error
}

// PolicyEnforcer checks whether claimed roles permit this operation.
// The real implementation is in internal/policy (M5). It does not
// authenticate the user or verify the role claims (see ADR-006).
type PolicyEnforcer interface {
	Authorize(req *contract.ExecutionRequest) error
}

// Translator maps a validated, authorized ExecutionRequest to a concrete
// system command and argument list. The real implementation is in
// internal/translation (M6).
type Translator interface {
	Translate(req *contract.ExecutionRequest) (command string, args []string, err error)
}

// Executor actually runs the translated command. The real bounded-process
// implementation is in internal/execution (M7); streaming arrives in M10.
type Executor interface {
	Execute(ctx context.Context, command string, args []string) (stdout, stderr string, exitCode int, err error)
}

// Core wires the four stages together into the fixed request lifecycle:
// Validate -> Authorize -> Translate -> Execute. All four dependencies are
// required; Core deliberately has no default values, so it is impossible
// to construct a Core that silently skips a stage.
type Core struct {
	Validator  Validator
	Policy     PolicyEnforcer
	Translator Translator
	Executor   Executor
}

// NewCore builds a Core from its four required stages.
func NewCore(v Validator, p PolicyEnforcer, t Translator, e Executor) *Core {
	return &Core{Validator: v, Policy: p, Translator: t, Executor: e}
}

// HandleRequest runs req through the full pipeline and always returns a
// non-nil ExecutionResponse — Core never panics or returns a bare error to
// the caller, since every outcome (including rejection) must be
// representable as a normal, auditable response (see REQ-002, AC-5).
func (c *Core) HandleRequest(ctx context.Context, req *contract.ExecutionRequest) *contract.ExecutionResponse {
	resp := &contract.ExecutionResponse{
		RequestID: req.RequestID,
		StartedAt: time.Now().UTC(),
	}

	if err := c.Validator.Validate(req); err != nil {
		return reject(resp, &StageError{Stage: StageValidation, Err: err})
	}

	if err := c.Policy.Authorize(req); err != nil {
		return reject(resp, &StageError{Stage: StagePolicy, Err: err})
	}

	command, args, err := c.Translator.Translate(req)
	if err != nil {
		return reject(resp, &StageError{Stage: StageTranslation, Err: err})
	}

	stdout, stderr, exitCode, err := c.Executor.Execute(ctx, command, args)
	resp.Stdout = stdout
	resp.Stderr = stderr
	resp.ExitCode = exitCode
	resp.EndedAt = time.Now().UTC()

	if err != nil {
		resp.Result = contract.ResultExecutionError
		resp.Message = (&StageError{Stage: StageExecution, Err: err}).Error()
		return resp
	}

	resp.Result = contract.ResultSuccess
	return resp
}

// reject finalizes a response as rejected at a given stage. Execution is
// never reached once reject is called — this is the enforcement point for
// "stop immediately at first failure" (REQ-002, AC-1..AC-4).
func reject(resp *contract.ExecutionResponse, err *StageError) *contract.ExecutionResponse {
	resp.EndedAt = time.Now().UTC()
	resp.Result = contract.ResultRejected
	resp.Message = err.Error()
	return resp
}
