# ADR-003: Interface-Based Pipeline Design for Agent Core

## Status
Accepted

## Context
When Agent Core was created in M2, the real implementations of Validation
(M4), Policy (M5), Translation (M6), and Execution (M7) did not yet exist.
Two options were available:

1. Wait until all four milestones are ready, then build Agent Core.
2. Define each stage contract as an interface immediately and build Agent Core
   around those contracts, allowing later milestones to supply one real
   implementation of each interface.

The first option conflicts with the project's Vertical Slice approach: build a
small but complete end-to-end path before completing every capability.

## Decision
- Define four independent interfaces: `Validator`, `PolicyEnforcer`,
  `Translator`, and `Executor`. `Core` implements none of them; it only invokes
  them in the fixed order Validate → Authorize → Translate → Execute.
- `Core.HandleRequest` stops immediately after the first failure and returns an
  `ExecutionResponse` with `Result = rejected`; it never calls the next stage.
- Errors are wrapped in `StageError{Stage, Err}` so the exact rejection stage is
  identifiable. This is essential for Audit (M9).
- To prove pipeline correctness in M2, placeholder implementations
  (`PassThroughValidator`, `AllowAllPolicy`, `IdentityTranslator`, and
  `NoOpExecutor`) live in a separate file (`placeholders.go`) with an explicit
  warning that they provide **no security guarantees** and exist only for
  pipeline testing.

## Alternatives Considered
1. **Implement stage logic directly inside Core without interfaces** — Rejected
   because Core would need rewrites in M4, M5, and M6, exactly the backtracking
   the design is meant to avoid.
2. **Use one generic `Stage` interface instead of four distinct interfaces** —
   Rejected because each stage has a different function signature. Translator
   returns command+args, while Executor returns stdout/stderr/exitCode. A
   generic interface would hide these differences and remove type safety.
3. **Put placeholders directly in tests rather than a separate file** —
   Rejected because it reduces readability and increases the chance of
   accidental reuse elsewhere, such as in an early demo. A separate file with
   an explicit warning reduces that risk.

## Consequences
- M4, M5, M6, and M7 only need to implement their corresponding interfaces and
  supply them to `NewCore(...)` in place of placeholders; `Core` itself remains unchanged.
- Tests in `core_test.go` use mocks that record every stage call. This proves
  early termination by verifying that later stages were never called, rather
  than inferring it only from the final result.
- No real Executor may be connected to Core until M4, M5, and M6 are complete.
  This is an operational rule, not a technical restriction, and must be
  enforced during review of subsequent milestones.
