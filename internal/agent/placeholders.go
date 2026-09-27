package agent

import (
	"context"

	"safeterm/internal/contract"
)

// ============================================================================
// ⚠️  INSECURE PLACEHOLDERS — M2 ONLY  ⚠️
//
// Everything in this file exists ONLY to prove that Core's pipeline wiring
// works end-to-end (REQ-002, AC-5) before the real stages exist. None of
// these types perform any real security enforcement:
//
//   - PassThroughValidator accepts every request without checking anything.
//   - AllowAllPolicy authorizes every request, regardless of role.
//   - IdentityTranslator passes the raw Command straight through as if it
//     were already a safe, concrete system command — this is exactly the
//     "raw shell" problem SafeTerm exists to prevent.
//
// They MUST be replaced by real implementations in M4 (Validation),
// M5 (Policy) and M6 (Translation) respectively, before Core is ever wired
// to a real Executor capable of touching the host system. Do not deploy,
// demo against real infrastructure, or reuse any of this outside of
// Core's own unit tests. See ADR-003 for the reasoning behind keeping
// these separate from — rather than baked into — Core itself.
// ============================================================================

// PassThroughValidator implements Validator by accepting everything.
type PassThroughValidator struct{}

func (PassThroughValidator) Validate(_ *contract.ExecutionRequest) error { return nil }

// AllowAllPolicy implements PolicyEnforcer by authorizing everything.
// Replaced by internal/policy.Enforcer in the M5 integration pipeline;
// retained only for isolated stage tests, including the M4 validation tests.
type AllowAllPolicy struct{}

func (AllowAllPolicy) Authorize(_ *contract.ExecutionRequest) error { return nil }

// IdentityTranslator implements Translator by returning the request's
// Command field unchanged, with no arguments. This is only safe for the
// M2 test/demo operation ("system_info") because NoOpExecutor (below)
// never actually calls exec() on it.
type IdentityTranslator struct{}

func (IdentityTranslator) Translate(req *contract.ExecutionRequest) (string, []string, error) {
	return req.Command, nil, nil
}

// NoOpExecutor implements Executor without ever invoking the operating
// system. It returns a canned success so Core's full lifecycle
// (Validate -> Authorize -> Translate -> Execute -> Respond) can be
// exercised safely in M2, with zero risk of running an arbitrary command
// on the developer's or CI machine.
type NoOpExecutor struct{}

func (NoOpExecutor) Execute(_ context.Context, command string, _ []string) (string, string, int, error) {
	return "ok: " + command + " (no-op executor, M2 placeholder)", "", 0, nil
}
