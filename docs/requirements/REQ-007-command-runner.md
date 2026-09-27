# REQ-007: Command Runner Must Execute Translated Commands Without a Shell

**Source:** M7 — Command Runner, the `agent.Executor` contract, and the
security boundary established by ADR-003 and ADR-007
**Priority:** Critical
**Status:** Done

## Description
The `internal/execution` package replaces `NoOpExecutor` in the M7 integration
path. It executes only the command and separate argument list supplied by the
Translation stage, using `exec.CommandContext(ctx, command, args...)` directly.
It must never construct or invoke a shell command string.

Every Runner has a positive configured timeout. It captures stdout and stderr
independently, reports the process exit code, and preserves output produced
before a failure, cancellation, or timeout. Validation, authorization, and
translation remain mandatory earlier stages; direct Runner use does not
establish that a command is allowed.

## Acceptance Criteria
- [x] **AC-1:** A successful process returns captured stdout and stderr,
  exit code 0, and no error.
- [x] **AC-2:** A process that starts but exits non-zero returns captured output,
  the exact exit code, and an error; Core maps it to `execution_error`.
- [x] **AC-3:** A command that cannot start returns exit code -1 and an error,
  without fabricated output or a panic.
- [x] **AC-4:** Every Runner requires a positive timeout. A process exceeding it
  is terminated, returns promptly with exit code -1, preserves prior output,
  and returns an error matching `context.DeadlineExceeded`.
- [x] **AC-5:** Parent-context cancellation terminates the process, returns exit
  code -1, preserves prior output, and returns an error matching `context.Canceled`.
- [x] **AC-6:** Arguments are passed as exact, separate argv elements, including
  whitespace, empty strings, quotes, and shell metacharacters. They are never
  joined, split, expanded, or interpreted by a shell.
- [x] **AC-7:** A nil context, empty command, and non-positive timeout fail
  safely. No process is started and construction never returns a usable Runner.
- [x] **AC-8:** The full real pipeline can validate, authorize, translate, and
  execute a configured test process while preserving request correlation,
  timestamps, stdout, stderr, and exit code.
- [x] **AC-9:** Unknown operations, raw executable names, invalid/injection
  arguments, and unauthorized roles are rejected before the real Executor is called.
- [x] **AC-10:** Execution timeout and non-zero process exit are reported as
  `execution_error`, never `rejected`; stdout/stderr/exit code are retained.

## Related Tests
- TEST-EXEC-001 through TEST-EXEC-007 (`internal/execution/runner_test.go`)
- TEST-INTEG-010 through TEST-INTEG-013 (`tests/integration/agent_execution_test.go`)

## Related ADRs
- ADR-003 (fixed pipeline and Executor interface)
- ADR-007 (separate command and argument tokens)
- ADR-008 (process execution, timeout, and exit semantics)
