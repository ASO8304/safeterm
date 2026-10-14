# SafeTerm

A configurable platform for secure system command execution.

## Current Status

**Milestone: M19 — Operator Catalog and Terminal UX (complete)**

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
- A hardened SSH Broker with hashed-password identity and optional public keys, server-owned catalog roles,
  an authorization-filtered operation menu, a fixed custom-shell grammar, PTY
  terminal behavior, streamed output, and Ctrl+C semantic termination
- Three least-privilege Config Manager profiles with authenticated Unix IPC,
  semantic resource/field APIs, preserving INI and deterministic JSON adapters,
  typed validation, redacted secrets, expiring proposals, optimistic concurrency,
  atomic writes, bounded backups, rollback, and optional typed activation checks
- An interactive `config` workflow in the SSH restricted terminal for listing,
  describing, reading, validating, previewing, applying, and rolling
  back registered fields without exposing a file editor or operating-system shell
- A repository threat model and dedicated adversarial suite covering injection,
  IPC confusion, privilege boundaries, hardlinks, catalog integrity, resource
  exhaustion, fuzz seeds, race detection, and secret-safe audit behavior
- Peer-credential policies that restrict each Config Manager UID to its own
  activation operations and reserve streaming access for the authenticated SSH Broker
- Continuous vulnerability, dependency, and CodeQL analysis in the security workflow
- A checksum-protected deployment bundle, strict preflight command, fresh-host
  systemd installer with dry-run support, and an automated disposable-host demo
- A public-boundary E2E test that authenticates over SSH and crosses real Unix
  IPC, Gateway, Runner, integrity, direct execution, and durable Audit boundaries
- A full restricted-shell line editor with cursor movement, insertion/deletion,
  bounded history, secret-safe input, and authorization-aware Tab completion
- Internal Console usernames/passwords behind one shared `safeterm` Linux login,
  with isolated Identity service and process-bound sessions across the signed
  catalog, Gateway, Runner, configuration and audit boundaries
- A `termctl` administration command with catalog, service, config, user, and
  shell-completion workflows
- A self-contained AMD64/ARM64 installation package that needs no Git, Go,
  module download, or Internet access on the target and preserves installed state on upgrade
- A checksum-protected pre-install transaction and offline rollback command that
  restores SafeTerm files, data, accounts, signing keys, and systemd state
- Commented catalog-authoring templates, allowlisted `tail -F` log aliases,
  role-separated typed systemd controls, and task-oriented terminal help

Policy does not authenticate users; it continues to authorize the roles carried
by the contract (ADR-006). The SSH Broker now authenticates its remote user and
is the trusted source of those roles for SSH traffic. Direct internal callers
must still treat arbitrary `ClaimedRoles` as unverified input.
The Agent Core pipeline, capability-isolated host runtime, local Audit Service,
runtime integrity boundary, bounded session control, authenticated SSH and
console Brokers, structured Config Managers, adversarial security controls,
reproducible deployment, transactional installation, and operator catalog
workflows are implemented through M19. The Node Gateway
remains local-only and accepts Brokers solely through authenticated Unix IPC.
M15 deployment evidence is recorded in [evidence/M15](evidence/M15/README.md).

Example translation output for trace with port=999 and host=192.168.10.20:

```text
command: /usr/bin/tcpdump
args: ["-l", "-nn", "-i", "any", "port", "999", "and", "host", "192.168.10.20"]
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
SSH client -> SSH Broker -----\
                              +-> Node Gateway -> diagnostics Runner
Local TTY -> Console Broker --/                -> network-observer Runner
                                    -> log-reader Runner
                                    -> service-manager Runner
                     -> application Config Manager
                     -> database Config Manager
                     -> network Config Manager
          Audit Service <- Gateway, every Runner, and every Config Manager
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
forwarding, environment, and non-session requests. A username and password authenticate against a salted bcrypt hash in a
root-managed identity file; public keys are optional per user. Both methods use
the same server-owned catalog roles. See [SSH authentication](docs/ssh-authentication.md). Connect through any SSH PTY
client, including OpenSSH or an Xterm-based frontend:

```bash
ssh -t -p 2222 alice@safeterm.example
```

Inside the terminal, use `list`, `describe NAME`, and
`execute NAME KEY=VALUE`. See [REQ-012](docs/requirements/REQ-012-ssh-broker-and-authentication.md),
[ADR-015](docs/decisions/ADR-015-restricted-ssh-broker.md), and the deployment
instructions in [deploy/systemd](deploy/systemd/README.md).

A structured configuration service is implemented in M13 according to
[ADR-014](docs/decisions/ADR-014-structured-configuration-management-service.md).
It uses resource IDs, typed format adapters, validated patches, atomic writes,
optimistic concurrency, preview-bound proposal tokens, rollback, and multiple
least-privilege service profiles. Broker renders the interactive menu or form;
the service never exposes arbitrary paths, replacement files, or a text editor.
Its acceptance criteria are in
[REQ-013](docs/requirements/REQ-013-structured-configuration-management.md),
with the terminal workflow and adapter grammar in
[Structured Configuration Management](docs/configuration-management.md).

M14 adds a maintained [threat model](docs/security/threat-model.md), a
repository-wide adversarial test package, bounded fuzz campaigns, race
detection, and hosted security analysis. It also closes the Config Manager
activation trust gap: Gateway authorizes each authenticated manager UID against
that manager's configured profile and accepts only argument-free typed systemd
operations registered in both the resource registry and signed operation
catalog. See [REQ-014](docs/requirements/REQ-014-adversarial-security-verification.md)
and [ADR-016](docs/decisions/ADR-016-adversarial-testing-and-peer-scoped-gateway.md).

M15 turns the implemented services into a reproducible fresh-host deployment.
`catalog-prepare` hashes the target host's exact executables, an offline operator
signs the normalized catalog, and `build-bundle.sh` creates a manifest-protected
runtime bundle. `safeterm-preflight` validates the bundle and installed host;
`install.sh` performs a non-mutating dry run before creating isolated accounts,
installing artifacts, and starting services in dependency order. The root-only
demo proves the deployed SSH terminal, verified operation, and structured config
menu. See [REQ-015](docs/requirements/REQ-015-end-to-end-deployment-and-demo.md),
[ADR-017](docs/decisions/ADR-017-signed-bundle-and-two-level-e2e-verification.md),
and the [deployment guide](docs/deployment.md).

M16 gives the restricted terminal normal line-editing behavior: cursor keys,
Home/End, insertion and deletion, bounded per-session history, and Tab
completion filtered through the user's authorized catalog and config views.
The same terminal is installed as the fixed login shell for the local
`safeterm` OS account. Internal Console authentication now uses an isolated
Identity service, bcrypt passwords and process-bound sessions verified by
Gateway and Config Managers (see [Console authentication](docs/console-authentication.md)
and [ADR-025](docs/decisions/ADR-025-internal-console-authentication.md)). It does not
start Bash or expose arbitrary command execution. See
[REQ-016](docs/requirements/REQ-016-interactive-terminal-and-console-broker.md),
[ADR-018](docs/decisions/ADR-018-shared-restricted-terminal-and-console-login.md),
and the [deployment guide](docs/deployment.md).

M17 packages prebuilt Linux services and catalog tools with all deployment
assets and a checksum manifest. The target prepares executable hashes locally,
then either consumes an externally signed production catalog or uses an
explicit single-host evaluation key. Fresh install and upgrade are both
supported; upgrades retain the SSH fingerprint, identities, resource registry,
and managed values. See [REQ-017](docs/requirements/REQ-017-offline-installation-package.md),
[ADR-019](docs/decisions/ADR-019-self-contained-offline-package.md), and the
[offline installation guide](docs/offline-installation.md).

Build a transferable AMD64 package on a connected Linux machine with:

```bash
make offline-package OUTPUT="$HOME/safeterm-offline-amd64" ARCH=amd64
```

M18 adds a verified transaction before every persistent package installation.
Rollback removes the current SafeTerm footprint, restores the previous files,
accounts, and service state, and works from either the installed administration
command or the package copy. See
[REQ-018](docs/requirements/REQ-018-transactional-installation-rollback.md),
[ADR-020](docs/decisions/ADR-020-transactional-installation-rollback.md), and the
[offline installation guide](docs/offline-installation.md).

```bash
sudo /usr/local/sbin/safeterm-install-rollback latest
```

M19 adds administrator-facing `TEMPLATE.jsonc` files, mapped parameters that
turn public aliases into fixed catalog values, an isolated streaming Log Reader,
and exact typed systemd controls backed by a generated no-wildcard sudoers
policy. The terminal accepts `execute log service_A`, completes mapped values, and
provides topic and operation help. See
[REQ-019](docs/requirements/REQ-019-operator-catalog-and-terminal-ux.md),
[ADR-021](docs/decisions/ADR-021-mapped-parameters-and-typed-service-control.md),
and the [catalog authoring guide](docs/catalog-authoring.md).

## Local Verification

```bash
make verify
make e2e
make security
```

`make verify` runs the formatting check, `go vet`, build, and unit/integration
tests in order. `make security` runs the dedicated adversarial suite and the
entire repository under Go's race detector. `make e2e` runs deployment preflight
and the real SSH-to-audit public-boundary test without requiring root or systemd.
When every check passes, the final output is:

```text
SafeTerm Verification: PASS
```

## Installation Packages

After initial installation, administrators can upgrade an extracted release
with `sudo termctl update /path/to/package`, restore the latest pre-install
state with `sudo termctl rollback`, or remove the runtime with
`sudo termctl uninstall`. Uninstall preserves data and accounts. See the
[termctl administration guide](docs/termctl.md).

GitHub Actions builds self-contained offline installation packages for Linux
AMD64 and ARM64. Tagged builds publish the latest packages as GitHub Release
assets:

- [Download AMD64 package](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-amd64.tar.gz)
- [Download AMD64 checksum](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-amd64.tar.gz.sha256)
- [Download ARM64 package](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-arm64.tar.gz)
- [Download ARM64 checksum](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-arm64.tar.gz.sha256)

On the target host, verify and extract the package before installation:

```bash
sha256sum -c safeterm-offline-amd64.tar.gz.sha256
tar -xzf safeterm-offline-amd64.tar.gz
cd safeterm-offline-amd64
sudo ./install.sh --help
```

For a single-host evaluation, create a bcrypt password hash and run the local
signing installation:

```bash
sudo install -d -m 700 /root/safeterm-secrets
sudo ./payload/bin/safeterm-password-hash \
  -output /root/safeterm-secrets/alice.hash

sudo ./install.sh \
  --local-signing \
  --ssh-user alice \
  --ssh-password-hash-file /root/safeterm-secrets/alice.hash \
  --single-operator \
  --dry-run

sudo ./install.sh \
  --local-signing \
  --ssh-user alice \
  --ssh-password-hash-file /root/safeterm-secrets/alice.hash \
  --single-operator
```

The complete air-gapped and production signing procedure is documented in
[docs/offline-installation.md](docs/offline-installation.md). Manual workflow
runs retain the same files as Actions artifacts; tagged runs additionally
publish them as release assets for the stable links above.

## Project Structure

The [catalog administration guide](configs/README.md) explains all catalog
sections in detail. A plain-text edition is in `configs/README.txt`; both
editions are installed at `/etc/safeterm/README.md` and
`/etc/safeterm/README.txt`.

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
| M13 | Structured Configuration Management | ✅ Tagged: v0.14.0-m13 |
| M14 | Adversarial Security Verification | ✅ Tagged: v0.15.1-m14 |
| M15 | End-to-End Deployment and Demo | ✅ Tagged: v0.16.0-m15 |
| M16 | Interactive Terminal UX and Console Access | ✅ Tagged: v0.17.0-m16 |
| M17 | Online and Air-Gapped Installation Package | ✅ Tagged: v0.18.0-m17 |
| M18 | Transactional Installation Rollback | ✅ Tagged: v0.19.0-m18 |
| M19 | Operator Catalog, Log Streaming, and Terminal UX | ✅ Tagged: v0.20.1-m19 |
