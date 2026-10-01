# M12 Verification Evidence

M12 implements the authenticated SSH Broker and restricted terminal defined by
[REQ-012](../../docs/requirements/REQ-012-ssh-broker-and-authentication.md) and
[ADR-015](../../docs/decisions/ADR-015-restricted-ssh-broker.md).

## Verified behavior

- Public-key authentication binds a unique key and SSH username to roles loaded
  from a strict, root-managed identity mapping.
- The Broker verifies the signed operation catalog and shows only operations
  authorized for those server-owned roles.
- A real SSH PTY session provides the SafeTerm custom shell without starting a
  host shell or PTY process.
- Raw SSH exec, subsystems, forwarding, environment changes, non-session
  channels, and multiple terminal channels are rejected.
- Bounded operations route through Gateway; long-running output streams to the
  terminal; Ctrl+C becomes `terminate(session_id)` for the authenticated owner.
- Untrusted command output is valid UTF-8 with terminal control characters
  neutralized before rendering.
- Identity and host-key files are bounded and opened with `O_NOFOLLOW` on Linux
  before descriptor-based type, size, and permission checks.
- The systemd unit runs as `safeterm-broker`, drops all capabilities, and has
  only network and authenticated Gateway Unix-socket access.

## Commands and results

```text
make verify
SafeTerm Verification: PASS

go test -race ./...
PASS (all packages)

GOOS=windows GOARCH=amd64 go build ./...
PASS
```

The complete verification logs are stored in
[make-verify.log](make-verify.log) and [race.log](race.log).

Tests `TEST-BROKER-001` through `TEST-BROKER-013` cover REQ-012. The live SSH
test performs a real key-authenticated handshake and PTY session and proves raw
exec rejection. Integration tests separately prove authenticated metadata is
routed through the real Gateway boundary and the deployment unit is hardened.
