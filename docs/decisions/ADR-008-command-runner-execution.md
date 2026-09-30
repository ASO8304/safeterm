# ADR-008: Direct Process Execution, Mandatory Timeout, and Exit Semantics

## Status
Accepted

## Context
M7 is the first milestone that starts operating-system processes. The Runner
must preserve the security property established in M6: a command and its
arguments remain distinct values. It also needs stable behavior for timeout,
context cancellation, process-start failure, non-zero exit, and captured output.

## Decision
- `internal/execution.Runner` implements the existing `agent.Executor`
  interface. Core and the interface signature remain unchanged.
- Runner invokes `exec.CommandContext(ctx, command, args...)` directly. It never
  calls `sh`, `bash`, `cmd.exe`, PowerShell, or any equivalent shell, and never
  joins arguments into a command string.
- Construction requires a positive timeout. Each execution derives a child
  context with that timeout, so the earlier of the caller's cancellation/
  deadline and the Runner timeout stops the process.
- Stdout and stderr use independent in-memory buffers. Their contents are
  returned on success and on every failure after process construction,
  including a non-zero exit, cancellation, and timeout.
- Exit code 0 represents success. If a started process exits normally with a
  non-zero status, that exact status is returned. `ExitCodeUnavailable = -1`
  represents failure to start, cancellation, timeout, or termination without
  a portable numeric exit status.
- Timeout errors wrap `context.DeadlineExceeded`; caller cancellation wraps
  `context.Canceled`. Other execution errors wrap the original `os/exec` error.
  Error messages include the command but not arguments, which may contain
  sensitive values.
- Empty commands and nil contexts are rejected before process construction.
- M7 does not define the deployment privilege boundary, restrict executable
  paths, resolve symlinks, verify hashes, authenticate roles, stream output, or
  cap captured output size. Capability-isolated services belong to M8,
  executable integrity to M10, streaming to M11, and adversarial resource
  testing to M14.

## Alternatives Considered
1. **Build a command string and execute it through a shell** — Rejected because
   shell parsing destroys the argv boundary and reintroduces command injection.
2. **Allow an unlimited runtime when timeout is zero** — Rejected because a
   configuration mistake could create an indefinitely running process. A
   positive timeout is mandatory.
3. **Return only an error and discard non-zero exit codes or partial output** —
   Rejected because Agent Core and future Audit need accurate execution evidence.
4. **Use external command-runner or process-management dependencies** — Rejected
   because the Go standard library provides the required behavior and the
   project deliberately avoids unnecessary dependencies.
5. **Enforce an executable allowlist again inside Runner** — Rejected because
   the real pipeline already derives the command from validated configuration
   through Translation. Integration tests prove earlier stages block untrusted
   operations before Runner. Path and binary integrity require M10's design.

## Consequences
- Runtime wiring must choose and document a timeout when constructing Runner.
- Callers can use `errors.Is` to distinguish deadline and cancellation errors.
- Capturing output in memory is acceptable for M7's bounded commands but must
  be revisited with explicit limits or streaming before long-running M11 workloads.
- `Runner` is a process-execution backend, not a license to deploy one
  all-powerful Agent. ADR-009 places it inside capability-isolated profiles.
- Tests use the Go test binary itself as a subprocess, avoiding shell commands
  and dependencies on host-specific utilities while exercising real process execution.
