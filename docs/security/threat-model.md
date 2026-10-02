# SafeTerm Threat Model

## Scope and assets
This model covers the SSH Broker, Node Gateway, capability-isolated Runners,
Config Managers, Audit Service, signed catalogs, root-owned identity/resource
registries, managed configuration files, Unix sockets, execution results, and
streaming session state on one Linux host.

Assets are authenticated identities and roles; approved operation semantics;
catalog signing trust; executable and configuration integrity; secret values;
audit ordering and durability; service availability; and the least-privilege
separation between profiles.

## Trust boundaries

```text
Untrusted SSH client
  -> SSH public-key authentication / restricted terminal
  -> Broker UID
     -> peer-scoped Gateway socket -> operation-specific Runner UID
     -> profile-specific Config Manager socket -> exact managed directory

Gateway, Runner, and Config Manager UIDs
  -> producer-scoped Audit socket -> append-only Audit UID/store

Offline signer -> root-owned public key + signed catalog
Root administrator -> SSH identities + Config Manager registry
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
| Forged remote identity or role | SSH key mapping creates server-owned roles; Gateway binds requests to allowed peer behavior | Live SSH tests, peer-policy tests, privilege-abuse suite |
| Cross-profile privilege escalation | Separate UIDs/sockets/catalog subsets/capabilities; peer-scoped activation operations | Routing and systemd tests; Config Manager Gateway denial tests |
| IPC parser confusion or memory exhaustion | Versioned strict JSON; unknown/trailing rejection; frame/deadline limits; fixed concurrent-connection admission ceilings | Malformed, oversized, and connection-saturation IPC tests plus fuzz seeds |
| Catalog/executable replacement | Detached signature; canonical path, mode, hash, native ELF, held descriptor | Tamper, symlink, interpreter, and replacement tests |
| Configuration traversal or corruption | Opaque resource IDs; canonical path rules; no links; schema/full-document validation; atomic rename | Path/link/parser/concurrency/rollback tests |
| Proposal theft, replay, or confused approval | Random short-lived single-use token bound to actor, patch, resource and base hash | Tamper, replay, expiry, actor and separate-approval tests |
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
