# M7 Evidence — Command Runner

Date: 2026-09-27. Environment: WSL, Go 1.22.2 on Linux/amd64.

## Baseline and Release Prerequisites
- PR #6 was squash-merged as commit `101b109` after both CI runs passed.
- Annotated tag `v0.6.0-m5` points to merged M5 commit `03b1fae`.
- Annotated tag `v0.7.0-m6` points to merged M6 commit `101b109`.
- Both tags were pushed to `origin` before M7 began.
- Branch `codex/m7-command-runner` was created from `origin/main` at `101b109`.
- The pre-change `make verify` run completed with `SafeTerm Verification: PASS`.

## Requirement and Decision
- REQ-007 defines ten acceptance criteria covering success, non-zero exit,
  start failure, timeout, caller cancellation, exact argv boundaries, invalid
  construction, full-pipeline execution, earlier-stage security gates, and
  Core result classification.
- ADR-008 requires direct `exec.CommandContext` execution with separate argv,
  a positive timeout, separate in-memory stdout/stderr capture, and stable exit semantics.
- `ExitCodeUnavailable = -1` represents start failure, cancellation, timeout,
  or termination without a portable exit code. A normally exited process
  preserves its exact non-zero status.
- Errors do not include argument values. They remain distinguishable with
  `errors.Is` for `context.DeadlineExceeded` and `context.Canceled`.

## Test Design
Unit and integration tests execute the Go test binary itself as a controlled
subprocess. This exercises real operating-system process creation without a
shell or dependency on host-specific commands.

- TEST-EXEC-001 through TEST-EXEC-007 cover AC-1 through AC-7.
- TEST-INTEG-010 verifies a successful M3–M7 pipeline and response correlation (AC-8).
- TEST-INTEG-011 verifies non-zero exit mapping and evidence preservation (AC-2, AC-10).
- TEST-INTEG-012 verifies timeout termination and evidence preservation (AC-4, AC-10).
- TEST-INTEG-013 proves unknown operations, raw executable names, unexpected or
  injection arguments, missing/unknown roles, and unauthorized roles never
  reach the real Executor (AC-9).

The unit argv test passes whitespace, an empty value, quotes, command
substitution syntax, a semicolon, a wildcard, and a newline as literal argv
elements and verifies that the child receives each value unchanged.

## Verification
[Complete make verify output](make-verify.log) is stored without abbreviation.

```text
SafeTerm Verification: PASS
```

[Race detector output](race.log) covers `internal/execution` and the integration
package. Both passed. Statement coverage for `internal/execution` is 100%.

## Reproduction

```sh
cd /mnt/c/Users/abolf/OneDrive/Documents/safeterm
GIT_CONFIG_COUNT=1 \
GIT_CONFIG_KEY_0=safe.directory \
GIT_CONFIG_VALUE_0=/mnt/c/Users/abolf/OneDrive/Documents/safeterm \
make verify

go test -race ./internal/execution ./tests/integration -count=1
```

## Limitations
- The production `cmd/agent` entry point remains a bootstrap because Brokers
  begin in M11. The real M7 stage is wired through integration tests.
- Runner inherits the parent process environment and resolves bare commands
  through normal `os/exec` behavior. Executable path/hash integrity belongs to M9.
- Output is captured in memory. Bounded output or streaming must be designed
  before M10 long-running operations are enabled.
- Authentication, audit logging, process-group management, privilege dropping,
  and adversarial resource testing are outside M7 and require later milestones.
