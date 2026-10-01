# SafeTerm

A configurable platform for secure system command execution.

## Current Status

**Milestone: M12 — Authenticated SSH Broker and Restricted Terminal (complete; tagged v0.13.0-m12)**

Available capabilities:

- Request/response contracts and a Core with the fixed Validation → Policy → Translation → Execution sequence
- JSON configuration loading and config-driven argument validation
- Authorization of claimed roles with the real Policy implementation and M5 integration tests
- Translation of operations into fixed config commands and separate arguments, without constructing shell strings
- Bounded process execution with separate argv elements, independent stdout/stderr capture, and exit-code reporting
- Independently deployable Gateway and capability-isolated Runner services with authenticated Unix IPC
- Declarative profile/executor mappings, an executable registry, atomic catalog snapshots, and typed systemd operations
- An independent, unprivileged Audit Service with authenticated local IPC,
  synchronous durable acknowledgement, sanitized events, and hash-linked JSONL storage
- Detached Ed25519 verification for active catalogs and profile-scoped SHA-256
  verification of native executables immediately before every execution
- Canonical-path and symlink-change detection with execution pinned to an open
  Linux `/proc/self/fd` descriptor, including the typed systemctl controller
- Versioned stdout/stderr streaming for approved long-running operations with
  ordered 16 KiB chunks, combined output budgets, bounded queues, idle and
  execution timeouts, and a configurable concurrent-session ceiling
- Opaque session routing and semantic termination with owner checks,
  idempotent states, dedicated process groups, graceful interrupt/terminate
  escalation, forced-kill fallback, and sanitized lifecycle audit events
- A hardened SSH Broker with public-key identity, server-owned catalog roles,
  an authorization-filtered operation menu, a fixed custom-shell grammar, PTY
  terminal behavior, streamed output, and Ctrl+C semantic termination

Policy does not authenticate users; it continues to authorize the roles carried
by the contract (ADR-006). The SSH Broker now authenticates its remote user and
is the trusted source of those roles for SSH traffic. Direct internal callers
must still treat arbitrary `ClaimedRoles` as unverified input.
The Agent Core pipeline, capability-isolated host runtime, local Audit Service,
runtime integrity boundary, bounded session control, and authenticated SSH
Broker are implemented through M12. The Node Gateway remains local-only and
accepts the Broker solely through authenticated Unix IPC. M12 evidence is
recorded in [evidence/M12](evidence/M12/README.md).

Example translation output for trace with port=999 and host=192.168.10.20:

```text
command: /usr/bin/tcpdump
args: ["-i", "any", "port", "999", "and", "host", "192.168.10.20"]
```

The `translation.New(cfg)` constructor returns an error that must be checked before
wiring the Translator into Core. A template accepts only a literal token or a complete
`{name}` placeholder, and every placeholder value must be present even when the
parameter is optional during Validation (ADR-007).

`execution.New(timeout)` requires a positive timeout and installs bounded output
defaults. Runner invokes `exec.Command(command, args...)` directly, never
through a shell, and manages a dedicated Linux process group itself so timeout,
disconnect, or semantic termination can apply the fixed escalation policy. It
returns `execution.ExitCodeUnavailable` (`-1`) when no portable process exit
status exists (ADR-008 and ADR-013).

## Target Service Architecture

SafeTerm does not deploy one universally privileged Agent. The Runner execution
backend runs inside capability-isolated service profiles.
A central Broker authenticates and routes requests to an unprivileged Node
Gateway; the Gateway performs no execution and forwards each operation over
host-local IPC to exactly one Runner profile.

```text
Client -> Broker -> Node Gateway -> diagnostics Runner
                                -> network-observer Runner
                                -> service-manager Runner
                                -> web-config Manager
                                -> database-config Manager
                                -> network-config Manager
                  Audit Service <- Gateway and every Runner
                                <- every Config Manager
```

Profiles share a codebase but run as separate processes with separate service
identities, configuration subsets, sockets, OS capabilities, filesystem access,
and resource limits. New profiles follow privilege boundaries rather than
individual commands. ADR-009 and REQ-008 define this target and its acceptance
criteria.

The Audit Service runs under a separate identity, accepts only authenticated
local producers, and has no execution capability. Gateway and Runner wait for
durable audit acknowledgement before crossing routing and execution boundaries.

Users invoke approved operations, not raw command lines. Authorized catalog
editors may propose declarative operations that reference an approved executor,
profile, and tokenized argument template. Activation is a separate authorized
step. New executables or wider privileges require stronger security approval.

Gateway and Runner accept only a catalog with a valid detached Ed25519
signature. Every registered executable includes the SHA-256 digest of the
exact target-host binary. Runner resolves and verifies that file at startup and
again immediately before execution, then executes the held file descriptor.
The shipped key/signature pair is development evidence; deployments generate
and protect their own offline signing key:

```bash
install -d -m 0700 "$HOME/.config/safeterm"

go run ./cmd/catalog-sign -generate-key \
  -private-key "$HOME/.config/safeterm/catalog-signing.key" \
  -public-key ./configs/trust/catalog-ed25519.pub

go run ./cmd/catalog-sign -config ./configs \
  -private-key "$HOME/.config/safeterm/catalog-signing.key" \
  -signature ./configs/catalog.sig
```

The private key must remain outside runtime hosts. Before signing, operators
replace example executable digests with hashes from the exact deployment host.
The key must reside on a filesystem that enforces Unix mode 0600; the signing
tool refuses group- or world-readable private keys.

M11 provides bounded stdout/stderr sessions for approved long-running operations.
Client-specific actions such as a web Terminate button or Ctrl+C in the SSH
custom shell map to one semantic `terminate` message. Runner owns process-group
cleanup and signal escalation; M11 does not expose a PTY or general shell. See
[REQ-011](docs/requirements/REQ-011-bounded-streaming-and-control.md) and
[ADR-013](docs/decisions/ADR-013-bounded-streaming-and-semantic-termination.md).

M12 provides a real SSH server whose session channel is the SafeTerm restricted
terminal. It does not invoke a login shell and rejects raw SSH exec, subsystem,
forwarding, environment, and non-session requests. Public keys map a username
to catalog roles in a root-managed identity file. Connect through any SSH PTY
client, including OpenSSH or an Xterm-based frontend:

```bash
ssh -t -p 2222 alice@safeterm.example
```

Inside the terminal, use `list`, `describe NAME`, and
`run NAME KEY=VALUE`. See [REQ-012](docs/requirements/REQ-012-ssh-broker-and-authentication.md),
[ADR-015](docs/decisions/ADR-015-restricted-ssh-broker.md), and the deployment
instructions in [deploy/systemd](deploy/systemd/README.md).

A structured configuration service is planned for M13 in
[ADR-014](docs/decisions/ADR-014-structured-configuration-management-service.md).
It uses resource IDs, typed format adapters, validated patches, atomic writes,
optimistic concurrency, preview-bound proposal tokens, rollback, and multiple
least-privilege service profiles. Broker renders the interactive menu or form;
the service never exposes arbitrary paths, replacement files, or a text editor.
Its acceptance criteria are in
[REQ-013](docs/requirements/REQ-013-structured-configuration-management.md).

## Local Verification

```bash
make verify
```

This command runs the formatting check, `go vet`, build, and unit/integration tests in order.
When every check passes, the final output is:

```text
SafeTerm Verification: PASS
```

## Project Structure

```text
safeterm/
├── cmd/            Executable entry points (gateway, runner, ...)
├── internal/       Internal logic (config, validation, policy, ...)
├── configs/        Configuration files (operations, roles, policies)
├── tests/          Integration, security, and end-to-end tests
├── docs/           Requirements, ADRs, and testing documentation
└── evidence/       Successful verification evidence for each milestone
```

## Development Methodology

Every new capability follows this cycle:

```text
Requirement → Design → Implementation → Unit Test →
Integration Test → Security Test → Evidence → Documentation → Git Tag
```

Details are recorded in `docs/decisions/` (Architecture Decision Records) and
`docs/requirements/`.

## Milestone Roadmap

| # | Title | Status |
|---|---|---|
| M0 | Project Foundation | ✅ Tagged: v0.1.0-m0 |
| M1 | ExecutionRequest Contract | ✅ Tagged: v0.2.0-m1 |
| M2 | Agent Core | ✅ Tagged: v0.3.0-m2 |
| M3 | Configuration Loading | ✅ Tagged: v0.4.0-m3 |
| M4 | Validation Engine | ✅ Tagged: v0.5.0-m4 |
| M5 | Policy Engine | ✅ Tagged: v0.6.0-m5 |
| M6 | Translation Layer | ✅ Tagged: v0.7.0-m6 |
| M7 | Command Runner | ✅ Tagged: v0.8.0-m7 |
| M8 | Capability-Isolated Runtime | ✅ Tagged: v0.9.0-m8 |
| M9 | Audit Service | ✅ Tagged: v0.10.0-m9 |
| M10 | Integrity Verification | ✅ Tagged: v0.11.0-m10 |
| M11 | Bounded Streaming and Semantic Termination | ✅ Tagged: v0.12.0-m11 |
| M12 | Authenticated SSH Broker and Restricted Terminal | ✅ Tagged: v0.13.0-m12 |
| M13 | Structured Configuration Management | ⏳ Planned |
| M14 | Security Testing | ⏳ Planned |
| M15 | End-to-End Deployment and Demo | ⏳ Planned |
