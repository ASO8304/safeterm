# REQ-002: Agent Core Must Process Every ExecutionRequest Through a Fixed, Stoppable Lifecycle

**Source:** Project specification sections 7.2 and 7.4 (Agent Core as the
central decision point), and section 8 (security model: Central Policy Enforcement)
**Priority:** Critical
**Status:** Done

## Description
Agent Core must process every `ExecutionRequest` in exactly this order:
**Validation → Policy → Translation → Execution**. If any of the first three
stages fails, no later stage, especially Execution, may be called.

Design note: when M2 was created, real implementations of Validation (M4),
Policy (M5), and Translation (M6) did not exist. The four stages were therefore
defined as interfaces (`Validator`, `PolicyEnforcer`, `Translator`, and
`Executor`). Agent Core guarantees ordering and early termination, not the
internal logic of each stage. M2 originally used permissive placeholders to
prove pipeline behavior. M14 removed those exported production symbols; the
equivalent test doubles now exist only in `_test.go` files. Runtime wiring uses
the real M4–M7 implementations.

## Acceptance Criteria
- [x] **AC-1:** If Validation fails, Policy, Translation, and Execution are never called
- [x] **AC-2:** If Policy fails, Translation and Execution are never called
- [x] **AC-3:** If Translation fails, Execution is never called
- [x] **AC-4:** If Execution fails, the result is `execution_error`, not `rejected`
  (distinguishing "SafeTerm refused to run it" from "it ran and the underlying command failed")
- [x] **AC-5:** When all stages succeed, the result is `success` and the response
  `RequestID` exactly matches the request `RequestID`
- [x] **AC-6:** `StartedAt` and `EndedAt` are populated for every path, successful
  or unsuccessful, because Audit (M9) requires duration for every operation
- [x] **AC-7:** Rejection and execution-error responses expose the typed failure
  stage as structured response data for Audit without changing pipeline order

## Related Tests
- TEST-AGENT-001 through TEST-AGENT-007 (`internal/agent/core_test.go`)

## Related ADRs
- ADR-003 (interface-based pipeline and StageError model)
