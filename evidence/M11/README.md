# M11 Verification Evidence

Milestone M11 adds bounded output streaming and semantic termination for
approved long-running operations without changing Agent Core or exposing a
general shell, PTY, PID, or caller-selected Unix signal.

## Delivered Controls

- Versioned session, stdout, stderr, exit, and terminate contracts use opaque
  192-bit session identifiers and strict bounded JSON frames.
- Gateway remembers the owning Runner and claimed local owner; Runner repeats
  profile, operation, owner, state, and long-running checks.
- stdout and stderr share a total output budget and emit ordered chunks through
  bounded queues. Concurrent sessions, idle time, and execution time are capped.
- Every process starts in a dedicated Linux process group. Cancellation sends
  SIGINT, then SIGTERM after a grace period, then SIGKILL after a second grace
  period. Descendants are covered by the same group.
- Web Terminate and SSH Ctrl+C adapters emit the same semantic `terminate`
  action. IPC rejects raw signals, unknown fields, profile overrides, and
  replacement argv.
- Client disconnect, output exhaustion, backpressure overflow, timeout, and
  idle timeout terminate the session without leaving an orphan process group.
- Session lifecycle events are synchronously sanitized for Audit Service; raw
  stream data and argument values are excluded.
- The end-to-end integration test passes through Gateway, authenticated Unix
  IPC, ProfileHandler, Core, and the M10 integrity-pinned Executor.

## Traceability

- Requirement: `docs/requirements/REQ-011-bounded-streaming-and-control.md`
- Design: `docs/decisions/ADR-013-bounded-streaming-and-semantic-termination.md`
- Tests: TEST-STREAM-001 through TEST-STREAM-024
- Matrix: `docs/testing/test-matrix.md`

## Verification Artifacts

- `make-verify.log` — formatting, vet, build, unit, integration, and coverage
  run ending in `SafeTerm Verification: PASS`.
- `race.log` — race-enabled tests for the entire repository.

## Operational Notes

- M11 authenticates local service peers with `SO_PEERCRED`. End-user identity is
  still a claim until M12, so the deployment must not expose this protocol as a
  secure remote per-user interface yet.
- Streaming is Linux-first because dedicated process groups and the integrity
  boundary rely on Linux behavior and `/proc`.
- Full-screen terminal applications, arbitrary stdin, Bash/sh, and PTY support
  remain outside the milestone.
