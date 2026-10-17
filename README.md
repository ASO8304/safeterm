# SafeTerm

A configurable platform for secure system command execution.

[Install a downloaded Release](#install-from-a-downloaded-github-release) ·
[Full installation and update guide](docs/offline-installation.md)

## Current Status

**Milestone: M19 — Operator Catalog and Terminal UX (complete)**

Available capabilities:

- External INI configuration updates through a separate restricted root writer,
  retaining file ownership, ACLs and comments; see [external configuration files](docs/external-configuration-files.md).

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

## Install from a downloaded GitHub Release

Use this procedure if you already downloaded a Release package. You do not need
to clone this repository or compile SafeTerm. Download links:

- [All Releases](https://github.com/ASO8304/safeterm/releases)
- [Download AMD64 package](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-amd64.tar.gz)
- [Download AMD64 checksum](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-amd64.tar.gz.sha256)
- [Download ARM64 package](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-arm64.tar.gz)
- [Download ARM64 checksum](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-arm64.tar.gz.sha256)

### 1. Choose the package for your server

Use Ubuntu 24.04 with systemd running as PID 1 and an administrator account with
`sudo`. Download **amd64** for `x86_64`, or **arm64** for `aarch64`/`arm64`. No Git,
Go or Python is required on the target. Examples below use amd64.

The guided installer detects missing OS tools and installs them from the configured
APT repositories on Ubuntu/Debian. On an air-gapped server, provide prerequisites
through your internal APT mirror or installation media first. The offline SafeTerm
package bundles SafeTerm binaries, not Ubuntu packages. A container without systemd
as PID 1 is not a supported target.

### 2. Verify and extract the downloaded archive

Run these commands in the directory containing the downloaded archive and checksum.
The new staging directory avoids mixing a new Release with an old extracted copy:

```bash
sha256sum -c safeterm-offline-amd64.tar.gz.sha256
release_dir=$(mktemp -d "$HOME/safeterm-release.XXXXXX")
tar -xzf safeterm-offline-amd64.tar.gz -C "$release_dir"
cd "$release_dir/safeterm-offline-amd64"
ls install.sh PACKAGE-SHA256SUMS TARGET_ARCH payload/bin/safeterm-termctl
```

Stop if checksum verification fails. Keep this shell in the extracted package
root for the following steps. If SafeTerm is **already installed**, skip fresh
password enrollment and use **Updating an existing installation** below.

### 3. Install with one command

This guided path requires a package containing ADR-028. Release v0.20.14 and older
retain their original explicit-flag installation procedure.

After extracting the archive, open a terminal in the extracted package directory
and run:

```bash
sudo ./install.sh
```

On a fresh server the installer asks:

```text
SafeTerm username [admin]:
Password:
Confirm password:
```

Press Enter to use `admin`, or choose another SafeTerm identity. The password is
hidden and must contain 8-72 printable ASCII characters, uppercase, lowercase and
a number. The installer hashes it directly; no manual hash file is needed and no
plaintext password is passed on a command line or saved in configuration.

The guided path uses local catalog signing. It installs missing prerequisites,
validates the prepared deployment, installs the services and `termctl`, and starts
SafeTerm. A successful installation ends with `SafeTerm package installation: PASS`
and login instructions. Package integrity, catalog verification and deployment
preflight run automatically; a separate preview command is optional.

The chosen identity is enrolled in both the SSH and Console user lists using the
password you entered. These are independent credentials afterward; changing one
does not change the other. No Linux account is created for this identity.
On an existing installation the same command updates the runtime while preserving
its verified catalog, users, passwords and managed values; it does not ask for a
new bootstrap password.

For a preview use `sudo ./install.sh --dry-run`. It does not install OS packages,
modify live configuration or start services; missing prerequisites must be
provisioned before preview can succeed. Fresh preview enrollment remains temporary.

Organizations requiring a signing key off the target should use the explicit
[production offline signing procedure](#production-offline-signing).

### 4. Verify services and test SSH

Run from the ordinary administrator shell:

```bash
command -v termctl
sudo /usr/local/bin/safeterm-preflight --installed
sudo termctl service status all
```

`termctl` is installed at `/usr/local/bin/termctl`. From another machine, replace
`SERVER_IP` with the target address:

```bash
ssh -tt -p 2222 admin@SERVER_IP
```

Check the displayed host-key fingerprint through a trusted channel before
accepting a new host. Enter the SSH password chosen during installation. If a host firewall or
network ACL is active, allow TCP port 2222 only from the intended client networks.

Inside the restricted SafeTerm terminal, try:

```text
whoami
execute ?
execute system_info
execute memory_info
execute ping host=127.0.0.1 count=5
config list
exit
```

Tab completes authorized inputs; a standalone `?` followed by Enter shows help.
Ctrl+C stops an active stream; Ctrl+] exits the whole SafeTerm session.

### 5. Test Console login and add users

After guided installation, open the shared restricted entry account:

```bash
sudo -iu safeterm
```

Enter the SafeTerm username and password chosen during installation. With the
default name, the resulting prompt is `safeterm::admin>`.

Console and SSH have independent user lists and passwords after initial enrollment.
Additional Console users can be enrolled separately:

```bash
sudo termctl user add console --id alice --roles viewer,netops,config-viewer
```

This asks for Alice's internal password; it does not create a Linux account named
`alice`. The Linux entry account `safeterm` is initially password-locked;
`sudo -iu safeterm` works for an authorized administrator. Direct physical Linux
login additionally requires a separate entry-account password set with
`sudo passwd safeterm`; the internal SafeTerm login remains mandatory.

Explicit unattended/key-only installs keep the disabled `local-admin` Console
placeholder unless an existing Console user database is present. Enroll Console
identities with `termctl user add console` in that mode.

To discover roles or change the internal password:

```bash
termctl role list
termctl role describe netops
sudo termctl user password console --id alice
```

### Unattended installation and existing automation

The guided path requires a terminal. For Ansible or fleet provisioning, retain
explicit signing and bootstrap options and use `--non-interactive`. Create a
protected bcrypt hash through your provisioning/secrets process; the bundled
helper can also create it interactively:

```bash
sudo ./payload/bin/safeterm-password-hash -output /root/admin-bootstrap.hash
sudo ./install.sh --non-interactive --local-signing --ssh-user admin \
  --ssh-password-hash-file /root/admin-bootstrap.hash
```

Add `--install-dependencies` to explicitly allow APT when provisioning without a
terminal; otherwise missing prerequisites produce an error instead of an implicit
package installation. Key-only bootstrap remains supported with
`--ssh-public-key FILE`. Public keys are optional in the password-based guided path.
Never put plaintext passwords into command arguments. Existing installation flags,
`--signed-catalog`, `--catalog-source`, `--no-start` and `--dry-run` remain supported.

### Updating an existing installation

Verify and extract the **new** archive using step 2, then run these commands from
its package root. For an installation using the default local signing key:

```bash
sudo termctl update "$PWD" --dry-run
sudo termctl update "$PWD"
sudo /usr/local/bin/safeterm-preflight --installed
sudo termctl service status all
```

`$PWD` must contain `install.sh`, `PACKAGE-SHA256SUMS`, `TARGET_ARCH` and `payload/`;
do not pass the archive, `payload/`, or the parent download directory. The update
preserves the installed signed catalog (including customized operations and log
aliases), identities, the SSH host key, the Config Manager registry and existing
managed files with their ownership and permissions. Release catalog defaults are
kept separately at `/usr/share/safeterm/defaults`. It restarts services and interrupts active sessions; use an
ordinary administrator session for the update.

An unchanged offline-signed catalog is also retained without a signing key. Only
when intentionally replacing the catalog with another reviewed signed version, use:

```bash
sudo termctl update "$PWD" --signed-catalog /path/to/signed-catalog --dry-run
sudo termctl update "$PWD" --signed-catalog /path/to/signed-catalog
```

To reuse customized operations and log aliases on other servers, see
[configuration preservation and portability](docs/configuration-portability.md).
Export with `sudo termctl catalog export --output /root/my-catalog`; on a compatible
locally signed target, use `sudo termctl catalog apply --source /root/my-catalog
--refresh-hashes --dry-run`, then repeat without `--dry-run`. This transfers
catalog definitions without cloning users or keys. This behavior requires a new
package containing ADR-027; older release installers are unchanged.

For an older Console OS-user mapping (for example, `user_id: safeterm`), set its
internal password once after upgrading; do not add the same ID again:

```bash
sudo termctl user password console --id safeterm
sudo -iu safeterm
```

Log in at the SafeTerm prompt using username `safeterm` and that internal password.
The old Linux account password does not replace internal authentication.

### Rollback and uninstall

From the ordinary administrator shell:

```bash
sudo termctl rollback
sudo termctl uninstall
```

These are separate actions: rollback restores the latest retained pre-install
snapshot; uninstall removes runtime binaries/units while retaining configuration,
accounts and audit data. Rollback also discards changes since the snapshot and
restores captured account databases, so use it before unrelated host account
changes. No transaction ID needs to be saved. See the full installation and
termctl guides for recovery after a partially failed installation.

For detailed offline signing, recovery and troubleshooting, see the
[full Release installation guide](docs/offline-installation.md). See also
[Console authentication and migration](docs/console-authentication.md) and
[termctl administration](docs/termctl.md). Manual Actions runs provide downloadable
artifacts; tagged runs publish the archive and checksum as Release assets.

## Apply catalog edits after installation

After adding log aliases or changing command definitions, run from the ordinary
administrator shell (with the existing local signing key):

```bash
sudo termctl catalog apply --dry-run
sudo termctl catalog apply
```

This validates, signs, saves a verified baseline, regenerates sudoers and activates
previously running services. Failed activation restores the earlier catalog;
`sudo termctl catalog rollback` also handles explicit rollback or interrupted
transactions. Users, passwords, managed settings and the trust anchor are not
replaced. For a working copy or offline signature, use `--source DIR` or
`--signed-catalog DIR`. See [catalog activation](docs/catalog-activation.md).

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
