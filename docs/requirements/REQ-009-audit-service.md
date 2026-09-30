# REQ-009: Audit Service Must Persist Correlated Runtime Decisions

**Source:** M9 — Audit Service, REQ-002 response correlation, and the lifecycle
events established by REQ-008 and ADR-010
**Priority:** Critical
**Status:** Done

## Description
SafeTerm must maintain an independent, durable audit trail across the Gateway
and capability-isolated Runners. The Audit Service runs as its own unprivileged
process and accepts events only from authenticated local service identities.

Audit events preserve the request ID, catalog version, component, profile,
operation, routing outcome, claimed identity context, response result, failure
stage, exit code, and timing data needed to reconstruct a request lifecycle.
Argument names are retained for diagnostic value, but raw argument values,
stdout, stderr, and free-form error messages are excluded because they can
contain credentials or other sensitive data. Claimed roles and user IDs remain
unverified claims until M12 authentication is implemented.

## Acceptance Criteria
- [x] **AC-1:** Gateway and Runner emit a versioned structured event containing
  one request ID, catalog version, component, profile, operation, outcome, and
  UTC occurrence time.
- [x] **AC-2:** Completion events record result, structured failure stage,
  execution exit code when applicable, start/end times, and duration without
  parsing an error message.
- [x] **AC-3:** Audit records include broker/user/claimed-role context and
  sorted argument names, but never raw argument values, stdout, stderr, or
  response messages.
- [x] **AC-4:** Audit runs as a separate service identity without execution
  capabilities and is the only service allowed to write the audit store.
- [x] **AC-5:** Producers and Audit Service mutually verify Linux Unix-socket
  peer UIDs. Each producer UID is authorized only for its component/profile;
  frames are bounded, strict JSON, and acknowledged only after acceptance.
- [x] **AC-6:** The store is append-only during service operation, mode 0600,
  synchronized before acknowledgement, safe for concurrent producers, and
  recoverable across restart without resetting sequence state.
- [x] **AC-7:** Every record has a monotonic sequence and SHA-256 link to the
  previous record. Startup/read validation rejects malformed, reordered,
  modified, internally deleted, incomplete, or symlink-backed stores.
- [x] **AC-8:** Failure to persist the pre-routing or pre-execution event stops
  the request before routing or execution. A completion-audit failure is
  returned to the caller rather than silently discarded.
- [x] **AC-9:** Integration tests prove one request produces a correlated
  Gateway/Runner trail over authenticated IPC and that a raw argument value is
  absent from the persisted bytes.

## Related Tests
- TEST-AUDIT-001 through TEST-AUDIT-008

## Related ADRs
- ADR-003 (fixed Agent Core pipeline and structured stage errors)
- ADR-006 (claimed identity is not authentication)
- ADR-009 (separate capability services)
- ADR-010 (authenticated local IPC and lifecycle events)
- ADR-011 (synchronous audit and hash-linked JSONL persistence)
