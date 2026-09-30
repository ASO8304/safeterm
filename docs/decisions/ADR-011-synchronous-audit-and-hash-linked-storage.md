# ADR-011: Synchronous Audit Service and Hash-Linked JSONL Storage

## Status
Accepted

## Context
M8 exposes transport-neutral Gateway and Runner lifecycle events, but does not
persist them. M9 needs one audit boundary that survives individual service
restarts, correlates distributed decisions, does not grant execution authority,
and makes silent event loss observable.

The design must also decide whether raw command arguments and output belong in
the audit trail, whether an unavailable audit sink permits execution, and how
to detect accidental or unauthorized modification without adding an external
database or dependency.

## Decision
- Audit is a separate `safeterm-audit` process and OS identity. It has no Agent
  Core, Translator, Executor, process capability, or network listener. Its only
  writable production path is the systemd-managed state directory.
- Gateway and Runner send events over bounded JSON frames on a Unix socket.
  Both ends verify `SO_PEERCRED` UIDs. Audit binds each producer UID to its
  allowed component and Runner profile, preventing one service from forging
  another service's event source. The service acknowledges an event only after
  validation, append, and `fsync` complete.
- Producers emit synchronously. Failure to record a routing or acceptance event
  stops the request before the next security boundary. Failure to record a
  completion event is returned to the caller and is never reported as success.
- The event contract contains correlation, catalog, routing, claimed identity,
  outcome, structured failure stage, exit code, and timing fields. It contains
  sorted argument names, but excludes argument values, stdout, stderr, and
  free-form messages to reduce credential and personal-data exposure.
- `ExecutionResponse` gains an optional `failure_stage` field. Agent Core sets
  it directly from its typed stage value, avoiding fragile parsing of `Message`.
  The four Core stage interfaces and their order remain unchanged.
- The service stores one strict JSON envelope per line. Envelopes have a
  monotonic sequence, the previous envelope hash, and a SHA-256 hash of their
  own canonical Go JSON representation. On startup, the full retained chain is
  verified before appending resumes.
- The hash chain detects modification, reordering, and internal deletion in the
  retained file. It is not a signature and cannot independently prove that the
  entire file was replaced or its tail truncated. Remote checkpoints and signed
  catalog evidence remain future control-plane work.
- The store rejects symbolic-link targets, incomplete final records, oversized
  records, unknown JSON fields, invalid events, and broken chains.

## Alternatives Considered
1. **Write audit lines independently from every Gateway and Runner** — Rejected
   because multiple privileged services would own writable audit state and
   ordering/recovery would be inconsistent.
2. **Buffer asynchronously and continue when audit is unavailable** — Rejected
   because a successful privileged action could have no durable evidence.
3. **Store raw arguments and command output** — Rejected because these fields
   can contain secrets and large or attacker-controlled data. Operation and
   argument names provide the required M9 traceability.
4. **Use syslog, journald, or an external database** — Deferred because each
   changes the deployment trust boundary or adds an external dependency. The
   Sink and service boundary allow a future backend without changing producers.
5. **Claim the SHA-256 chain is tamper-proof** — Rejected. Without a protected
   signing key or external checkpoint, a writer with full file access can
   recompute or replace the chain.

## Consequences
- Audit availability is part of request availability. Operators must supervise
  Audit Service before Gateway and Runner units.
- Every producer blocks briefly for durable acknowledgement at each lifecycle
  point. M11 streaming may batch non-critical progress events, but start and
  completion records remain durability boundaries.
- M12 must replace claimed identity fields with authenticated Broker context
  while retaining the same request ID.
- Retention, export, external checkpointing, and access-control policy for audit
  readers require deployment work beyond the local M9 store.
