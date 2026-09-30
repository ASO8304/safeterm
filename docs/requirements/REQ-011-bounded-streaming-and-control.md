# REQ-011: Long-Running Operations Must Support Bounded Streaming and Semantic Termination

**Source:** M11 — Bounded Streaming and Semantic Termination, ADR-008 bounded
execution, ADR-009 capability-isolated Runners, and ADR-010 local IPC
**Priority:** Critical
**Status:** Planned

## Description
SafeTerm must stream bounded stdout and stderr from approved long-running
operations without exposing a general shell, arbitrary standard input, process
identifiers, or operating-system signals to callers. A Broker presents the
interaction appropriate to its client: a web adapter may expose a Terminate
button, while an SSH custom-shell adapter may translate Ctrl+C. Both emit the
same transport-neutral `terminate` action for an opaque execution session.

The Runner that owns the session remains responsible for the process lifecycle.
It starts the command in a dedicated process group, records a state transition
before delivering an interrupt, and escalates after bounded grace periods when
the operation does not exit. Gateway only routes stream and control frames; it
does not select signals, translate commands, or execute a fallback process.

This milestone does not add a PTY, an interactive Bash or sh session, arbitrary
stdin forwarding, or editor-style terminal emulation. Structured workflows such
as configuration editing use semantic application messages rather than a PTY.

## Acceptance Criteria
- [ ] **AC-1:** Starting an approved operation marked `long_running` returns an
  unpredictable session ID distinct from the request ID and emits bounded,
  ordered stdout and stderr chunks.
- [ ] **AC-2:** Existing short-running operations retain compatible bounded
  request/response behavior.
- [ ] **AC-3:** Client adapters send the semantic action `terminate` with a
  session ID. Public contracts never accept a PID, process-group ID, raw signal,
  or caller-selected escalation policy.
- [ ] **AC-4:** Gateway routes a control action only to the Runner profile that
  owns the session. Runner independently checks the session, profile, and
  authorized owner before changing process state.
- [ ] **AC-5:** Runner starts each long-running operation in an isolated process
  group and terminates the whole group using SIGINT, a configured grace period,
  SIGTERM, a second bounded grace period, and finally SIGKILL when required.
- [ ] **AC-6:** Session states follow `starting`, `running`, `terminating`, and
  `exited`. Repeated terminate requests are idempotent; late requests return
  `already_exited`, and unknown sessions return `not_found` without affecting
  another execution.
- [ ] **AC-7:** Stream frames have sequence numbers, explicit stdout/stderr
  channels, strict size limits, a per-session output budget, and bounded
  backpressure. A slow consumer cannot cause unbounded memory growth.
- [ ] **AC-8:** Execution timeout, idle timeout, client disconnect, output-budget
  exhaustion, and backpressure overflow have explicit fail-closed behavior and
  leave no orphaned process group.
- [ ] **AC-9:** Stream and control frames are versioned and size-bounded. They
  cannot carry translated argv, override a profile, or bypass the catalog,
  Policy, Translation, integrity, or audit boundaries.
- [ ] **AC-10:** Catalog and executable integrity are verified before process
  creation exactly as in M10. A streaming path cannot weaken descriptor-pinned
  execution or Core's fixed stage ordering.
- [ ] **AC-11:** Audit records session start, stream open, terminate request,
  interrupt delivery, grace-period expiry, forced kill, and exit metadata. It
  excludes raw stream contents and validated argument values.
- [ ] **AC-12:** Integration tests prove that web-button and SSH Ctrl+C adapters
  converge on the same control action, the correct process group terminates,
  repeated and late termination are stable, and cross-session control is denied.
- [ ] **AC-13:** Agent Core retains its four interfaces and fixed pipeline.
  Streaming is implemented in Runner session orchestration with a bounded sink
  and cancellable execution context.

## Related Tests
- Planned for M11.

## Related ADRs
- ADR-008 (direct bounded process execution)
- ADR-009 (capability-isolated Runner services)
- ADR-010 (authenticated local IPC)
- ADR-011 (audit lifecycle events)
- ADR-012 (pinned executable integrity)
- ADR-013 (bounded streaming and semantic termination)
