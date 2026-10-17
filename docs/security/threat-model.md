# SafeTerm Threat Model

## Scope and assets
This model covers the SSH Broker, local Console Broker, Node Gateway, capability-isolated Runners,
Config Managers, isolated Console Identity service, Audit Service, signed catalogs, root-owned identity/resource
registries, managed configuration files, Unix sockets, execution results, and
streaming session state on one Linux host.

Assets are authenticated identities and roles; approved operation semantics;
catalog signing trust; executable and configuration integrity; secret values;
audit ordering and durability; service availability; and the least-privilege
separation between profiles.

## Trust boundaries

```text
Untrusted SSH client
  -> SSH bcrypt-password or optional public-key authentication / restricted terminal
  -> Broker UID
     -> peer-scoped Gateway socket -> operation-specific Runner UID
     -> profile-specific Config Manager socket -> exact managed directory

Local console login
  -> shared Linux entry account / fixed SafeTerm login shell
  -> internal bcrypt login at isolated Identity service
  -> process-bound session -> Gateway and Config Managers verify session + derive roles

Gateway, Runner, and Config Manager UIDs
  -> producer-scoped Audit socket -> append-only Audit UID/store

Offline signer -> root-owned public key + signed catalog
Root administrator -> SSH/console identities + Config Manager registry
```

The network, terminal bytes, command names, arguments, configuration candidate
values, claimed protocol fields, file paths encountered through the filesystem,
and process output are hostile. Root, the offline signing host, the kernel, and
the installed SafeTerm binaries are trusted. A compromised unprivileged service
is assumed able to send arbitrary bytes as its UID; the design limits it to that
UID's peer policy, socket access, filesystem view, capabilities, and cgroup.

## Invariants and verification

| Threat | Required control | Verification |
|---|---|---|
| Shell/argument injection | Whitelisted operations and parameters; whole-token templates; direct argv execution; no shell | Injection corpus, fuzz seeds, AST/source invariant |
| Forged identity or role | SSH credential authentication or internal Console bcrypt login; Console sessions bound to kernel UID/PID, verified at Gateway and Config Managers with authoritative roles | Live SSH tests, identity/session replay and role-forgery tests, peer-policy tests, privilege-abuse suite |
| Cross-profile privilege escalation | Separate UIDs/sockets/catalog subsets/capabilities; peer-scoped activation operations | Routing and systemd tests; Config Manager Gateway denial tests |
| IPC parser confusion or memory exhaustion | Versioned strict JSON; unknown/trailing rejection; frame/deadline limits; fixed concurrent-connection admission ceilings | Malformed, oversized, and connection-saturation IPC tests plus fuzz seeds |
| Catalog/executable replacement | Detached signature; canonical path, mode, hash, native ELF, held descriptor | Tamper, symlink, interpreter, and replacement tests |
| Configuration traversal or corruption | Opaque resource IDs; canonical path rules; no links; schema/full-document validation; atomic rename | Path/link/parser/concurrency/rollback tests |
| Proposal theft, replay, or confused approval | Random short-lived single-use token bound to actor, patch, resource and base hash | Tamper, replay, expiry, actor/session binding tests |
| Secret or control-byte disclosure | Redacted fields/diffs/audit; argument names only; terminal byte filtering | Secret corpus, audit inspection, terminal sanitization tests |
| Process/output/session exhaustion | Deadlines, output/queue/session/connection limits, process groups, finite cgroup/systemd bounds | Resource-abuse, manifest-value, and lifecycle tests; race detector |
| Audit spoofing or tampering | Producer UID/profile policy; synchronous fsync; hash-linked records; single-link store | Producer-spoof, corruption, partial-write, hard-link tests |
| Vulnerable dependency or unsafe dataflow | Minimal dependencies, checksum verification, govulncheck, CodeQL, and platform dependency review when available | Required Security CI jobs and retained SARIF |

## Exclusions and residual risk
SafeTerm does not defend against a malicious kernel/root administrator, theft of
the offline signing key, modification of the installed SafeTerm binary before
startup, hardware compromise, or denial of the entire host. The local hash
chain detects record mutation/reordering within the retained file but cannot
prove that root did not replace or truncate the complete audit store; external
checkpoints remain a deployment responsibility. Availability still depends on
systemd/cgroup limits and host capacity. Independent penetration testing and
production monitoring remain required before high-assurance deployment.

## SSH password authentication

The root-managed SSH identity map stores salted bcrypt hashes, never plaintext
passwords. New hashes use cost 12; accepted work factors are bounded to 10–14.
Password input is limited to 72 bytes and is never included in errors, terminal
output, operation requests, or audit events. The identity map must not be
world-accessible (installed mode 0640, root:safeterm-broker). The same configured
roles apply to password and key authentication; neither method reads OS passwords.
An unknown username performs a dummy bcrypt comparison. Expensive password
comparisons are capped at four concurrent requests, SSH allows three attempts
per connection, and the existing connection ceiling and handshake deadline apply.
These bounds limit CPU work; they are not a per-source rate limiter or an account
lockout policy. Deployments exposed to untrusted networks should apply network
access controls. Public keys are offered only when a user has configured them.

Internal Console users no longer require per-person OS accounts. The shared entry
UID cannot read credential hashes or use verification-only Identity APIs. Legacy
OS mappings without bcrypt hashes are denied. Login rate/concurrency bounds,
short-lived opaque sessions, stream revalidation and expiry cancellation limit
abuse. Processes sharing that UID can still deny service; never grant it a general
OS shell. See [Console authentication](../console-authentication.md) and ADR-025.


## Restricted external configuration writer (ADR-029)

The dedicated root file writer is a new trusted boundary. Config Managers remain
unprivileged and retain user-role/proposal authorization. The writer trusts only
profile-mapped manager UIDs over Unix peer credentials, revalidates registered
resource/field patches and versions, and accepts no caller path, rendered document
or command. A compromised manager can access registered files and editable fields
in its own profile, not arbitrary root files. Root-authored resource schemas are
security policy and must constrain sensitive values appropriately.

The writer's root-controlled parent namespace, protected-target denylist, generated
systemd directory grants, link/type checks, bounded IPC, exact metadata checks and
independent audit reduce the scope of that privilege. It uses only CHOWN, FOWNER
and DAC_READ_SEARCH capabilities, has no subprocess interface, and fails when
ACL/xattr/label preservation is unavailable. Root-private backups remain outside
application directories. Atomic replacement changes inode/mtime. An external
application ignoring deployment coordination can still race an optimistic update;
version/inode/metadata rechecks detect changes but do not supply a universal OS
transaction against other independent writers.
