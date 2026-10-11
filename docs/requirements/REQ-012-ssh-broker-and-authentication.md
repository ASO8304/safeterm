# REQ-012: Authenticated SSH Broker and Restricted Terminal

**Source:** M12 roadmap and the approved SSH restricted-terminal design
**Priority:** Critical
**Status:** Done

## Description

SafeTerm must expose an SSH service whose interactive session is a restricted
operation terminal rather than a general-purpose operating-system shell. The
Broker authenticates a configured user with a public key, obtains roles from a
root-managed identity mapping, presents only catalog operations authorized for
those roles, and creates the existing `ExecutionRequest` contract itself.

The SSH transport may allocate a PTY for terminal behavior, but it must not
start a host PTY, shell, command interpreter, or arbitrary process. Approved
bounded and long-running operations continue through the authenticated local
Gateway IPC. During a long-running stream, Ctrl+C becomes the transport-neutral
semantic `terminate` action defined by REQ-011.

## Acceptance Criteria

- [x] **AC-1:** Only SSH public-key authentication is enabled; username and
  key must match one configured identity.
- [x] **AC-2:** Roles come only from the Broker identity file, are validated
  against the signed catalog, and are injected into request metadata.
- [x] **AC-3:** The terminal lists and describes only operations authorized by
  the authenticated user's roles.
- [x] **AC-4:** The accepted grammar is limited to `help`, `list`, `describe`,
  `execute`, `whoami`, `clear`, and session-exit commands. Operation arguments use
  bounded `KEY=VALUE` tokens and still pass Core validation.
- [x] **AC-5:** SSH `exec`, subsystem, forwarding, environment, agent-forwarding,
  and non-session channels are rejected. One PTY terminal is allowed per SSH
  connection.
- [x] **AC-6:** No host shell or executable is started by the Broker; it can
  communicate only with the Node Gateway through authenticated Unix IPC.
- [x] **AC-7:** stdout/stderr stream to the SSH channel and Ctrl+C sends only
  `terminate(session_id)` with the authenticated owner.
- [x] **AC-8:** Identity and host-key files are bounded, strict, non-symlink
  files with secure Unix write/read modes, and malformed or duplicate entries
  fail startup.
- [x] **AC-9:** Handshake time, authentication attempts, input lines,
  connections, terminal dimensions, and protocol payloads have explicit
  bounds.
- [x] **AC-10:** The signed catalog is verified before the Broker starts and
  the service runs as an unprivileged, hardened `safeterm-broker` account.
- [x] **AC-11:** Unit, live SSH transport, and integration tests prove identity
  binding, authorization filtering, raw-exec rejection, routing, streaming,
  and semantic termination.
- [x] **AC-12:** Command output is converted to valid UTF-8 and terminal control
  characters are neutralized before data reaches the SSH terminal.

## Related Tests

- TEST-BROKER-001 through TEST-BROKER-013

## Related ADRs

- ADR-006
- ADR-009
- ADR-010
- ADR-013
- ADR-015
