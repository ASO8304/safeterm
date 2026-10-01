# ADR-015: Restricted SSH Broker and Public-Key Identity

## Status

Accepted

## Context

M12 needs the first remote Broker. Users expect an SSH terminal experience,
including a PTY, line editing, streamed output, and Ctrl+C. Giving the Broker a
login shell, running commands received through SSH `exec`, or attaching a host
PTY would create an escape path around SafeTerm's catalog, policy, translation,
Runner isolation, and audit boundaries. The Go standard library does not
implement the SSH protocol.

## Decision

- Add `golang.org/x/crypto/ssh` v0.33.0, the newest release compatible with the
  project's Go 1.22 baseline. The dependency is limited to SSH transport and
  cryptography; operation parsing and all SafeTerm policy remain project code.
- Run `cmd/ssh-broker` as the unprivileged `safeterm-broker` service. It verifies
  the same signed catalog used by Gateway and Runner, then connects only to the
  authenticated Gateway Unix socket.
- Authenticate with public keys only. A root-managed JSON file maps a unique
  SSH key fingerprint and username to catalog-defined roles. The remote client
  cannot submit or replace roles. Linux opens both the identity file and host
  key with `O_NOFOLLOW`, then checks size, type, and permissions on the opened
  descriptor before reading it.
- Provide an SSH PTY as a terminal protocol feature without creating an OS PTY.
  The custom shell accepts a small fixed command grammar and never invokes a
  shell or local executable.
- Reject SSH exec, subsystems, forwarding, environment changes, arbitrary
  channels, and a second session on the same connection. Bound authentication
  attempts, handshake duration, concurrent connections, input length, PTY
  metadata, and operation streams.
- Convert Ctrl+C during a stream to the REQ-011 semantic termination contract.
  Signal selection and process ownership stay inside the responsible Runner.
- Neutralize terminal control characters in command output before rendering it;
  only the Broker's own fixed UI may emit terminal escape sequences.
- Xterm, OpenSSH, and other SSH terminal clients remain client-side choices;
  the Broker does not embed a terminal emulator.

## Alternatives Considered

1. **Use OpenSSH `ForceCommand` around a local shell binary** — Rejected because
   deployment correctness would depend on sshd configuration and would leave
   more ambient SSH features outside the Broker's explicit allowlist.
2. **Allocate a host PTY and run Bash with a restricted PATH** — Rejected
   because shell built-ins, expansions, interpreters, and newly installed
   programs make PATH restriction an inadequate security boundary.
3. **Implement SSH from the RFCs with only the standard library** — Rejected
   because a new cryptographic network protocol implementation would be much
   larger and riskier than the maintained Go SSH package.
4. **Accept passwords** — Rejected because SafeTerm would need password storage,
   throttling, recovery, and secret-handling controls that public keys avoid.
5. **Trust roles sent by the client** — Rejected because it would preserve the
   unverified-claim weakness documented by ADR-006.

## Consequences

- SSH users receive a familiar interactive terminal while every executable
  still passes through the existing SafeTerm security pipeline.
- The SSH library becomes the first external Go dependency and must be included
  in dependency and vulnerability maintenance.
- Identity changes require a Broker restart. Central identity providers,
  short-lived SSH certificates, Web adapters, and catalog proposal/approval
  UX can be added later without changing the authenticated principal or
  semantic-control contracts.
