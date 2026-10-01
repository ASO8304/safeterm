# ADR-013: Bounded Streaming and Semantic Termination

## Status
Accepted

## Context
Some approved operations, such as packet capture, produce output continuously
and must remain active until the user stops them. A web Broker naturally exposes
a button, while an SSH custom shell naturally receives Ctrl+C. Passing a raw
signal or PID through either interface would couple clients to Linux process
details and let callers influence a privileged lifecycle decision.

M10 execution buffers output until completion. ADR-010 local IPC permits one
bounded request and one response per connection. M11 must add streaming and
control while preserving capability-specific Runner isolation, the fixed Agent
Core pipeline, signed catalogs, executable pinning, and synchronous audit.

## Decision
- M11 introduces a versioned, bounded session protocol with semantic messages:
  start, session-started, stdout/stderr chunk, terminate, and session-exited.
  Each frame has a maximum size and each stream chunk has a monotonically
  increasing sequence number.
- A session ID is an unpredictable capability reference distinct from the
  request ID. Gateway records which Runner connection owns it and cannot reroute
  it to another profile. Runner remains the authoritative session registry.
- Broker adapters own user-interface conventions. A web adapter maps a
  Terminate button to `terminate(session_id)`; an SSH custom-shell adapter
  intercepts Ctrl+C and maps it to the same action. No adapter sends a Unix
  signal, PID, process group, or replacement argv.
- Runner validates that the requester may control the session, records the
  transition to `terminating`, and applies its fixed lifecycle policy to the
  entire dedicated process group: SIGINT, bounded grace period, SIGTERM, second
  bounded grace period, then SIGKILL. The public action remains `terminate`
  regardless of which internal step completes the stop.
- Session states are `starting`, `running`, `terminating`, and `exited`.
  Termination is idempotent. A request for an exited session returns
  `already_exited`; an unknown ID returns `not_found`.
- Streaming has configured chunk limits, a total output budget, execution and
  idle timeouts, bounded queues, and a concurrent-session ceiling. Backpressure
  cannot grow memory without a limit. Overflow and disconnect policy must
  terminate or drain a session in a defined way and cannot leave an orphaned
  process group.
- Output content is delivered to the authorized client but is not copied into
  audit storage. Audit records lifecycle metadata including session start,
  stream open, terminate request, delivered interrupt, grace expiry, forced
  kill, and final exit.
- Core keeps the four interfaces and the Validation → Policy → Translation →
  Execution ordering. Runner orchestration supplies a bounded stream sink and
  cancellable session context around execution; Gateway gains routing, not an
  Executor.
- M11 does not provide a PTY, terminal emulation, a Bash/sh session, arbitrary
  stdin, or interactive full-screen programs such as Vim. Application-specific
  interactions use typed semantic messages.
- Authenticated end-user ownership arrives with M12. Before that identity is
  available, the local M11 protocol may be tested with authenticated service
  peers but must not claim secure remote per-user cancellation.

## Alternatives Considered
1. **Forward Ctrl+C as SIGINT from every client** — Rejected because clients
   should express intent while Runner owns operating-system process policy.
2. **Expose PID and a generic signal operation** — Rejected because it leaks a
   privileged primitive, creates PID-reuse risks, and permits policy bypass.
3. **Allocate a PTY for every long-running operation** — Rejected because packet
   capture streaming and semantic termination do not require terminal emulation,
   and a PTY greatly expands input and escape-sequence handling.
4. **Let Gateway terminate local processes** — Rejected because Gateway has no
   Executor and must not share Runner privileges.
5. **Buffer output until process exit** — Rejected because it does not support
   observation or control of long-running operations and can exhaust memory.
6. **Kill only the direct child** — Rejected because descendants could survive
   and retain the Runner's capability.

## Consequences
- Web and SSH experiences can differ while sharing one audited control contract.
- Runner needs a concurrent session registry, process-group lifecycle handling,
  and deterministic cleanup for disconnect and timeout cases.
- Local IPC needs bounded multiplexed stream/control framing in place of the M8
  one-request/one-response connection for long-running sessions.
- Short-running operation behavior remains compatible and need not allocate a
  long-lived session after its final response.
- Full-screen terminal programs remain outside the approved product scope until
  a separate PTY threat model and ADR are accepted.
