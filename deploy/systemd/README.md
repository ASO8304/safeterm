# Linux systemd deployment

SafeTerm ships authenticated restricted SSH and local-console Brokers, three isolated Config
Managers, an isolated Console Identity service, and an independent append-only Audit Service alongside one unprivileged
Node Gateway and four capability-isolated Runner units. All Runner units use
the same `safeterm-runner` binary, but they run as
different users with separate runtime directories, sockets, catalogs, resource
limits, and capability bounding sets.

Use the automated bundle, preflight, installer, and demo procedures in the
[deployment guide](../../docs/deployment.md). The remainder of this document
describes the service identities and security boundaries enforced by the units.

Required service accounts:

```text
safeterm-identity
safeterm-gateway
safeterm-broker
safeterm
safeterm-diagnostics
safeterm-network
safeterm-log-reader
safeterm-service-manager
safeterm-audit
safeterm-config-application
safeterm-config-database
safeterm-config-network
```

The Runner accounts use `safeterm-gateway` as their service group so the
Gateway can reach their socket directories. Runner servers still verify the
Gateway UID with `SO_PEERCRED`; group membership alone cannot authorize a
request. The Gateway similarly verifies each Runner UID. The SSH Broker and
console account have `safeterm-gateway` as a supplementary group so they can
reach the Gateway socket; Gateway independently verifies each Broker UID before
accepting a request. Gateway applies a distinct peer policy after that identity
check: each Broker may use unary and streaming routes only with its bound
`broker_type` (`ssh` or `console`). Console requests additionally require a
process-bound internal session; Gateway replaces caller roles with Identity
service roles. The Identity socket permits login/logout only from the shared
Console UID and verification only from Gateway/Config Manager UIDs. Its 8 KiB
protocol admits at most 32 concurrent connections. Each
Config Manager UID may use unary routing only for the typed activation and
health operations assigned to its own profile.

Gateway and Runner accounts also belong to the `safeterm-audit` group so they
can reach the Audit Service socket. The Audit Service independently verifies
each producer UID with `SO_PEERCRED`, runs without execution capabilities, and
writes only to `/var/lib/safeterm/audit/audit.jsonl`. Audit records contain argument
names but never raw argument values. Every append is synchronized before the
producer receives acknowledgement, and each record is linked to the previous
record by SHA-256 to detect modification, reordering, and deletion inside the
retained chain. The hash chain is corruption evidence, not a digital signature
or an external checkpoint against complete-file replacement or tail truncation.

Install the binaries as `/usr/local/bin/safeterm-ssh-broker`, `/usr/local/bin/safeterm-console`, `/usr/local/bin/safeterm-config-manager`, `/usr/local/bin/safeterm-gateway`,
`/usr/local/bin/safeterm-runner`, `/usr/local/bin/safeterm-audit`,
`/usr/local/bin/safeterm-identity`, and keep
`catalog-sign` only on an offline administration host. Install the catalog,
`catalog.sig`, and the separately provisioned public key at
`/etc/safeterm/trust/catalog-ed25519.pub` read-only, then copy the unit files to
`/etc/systemd/system`.

Generate a dedicated Ed25519 SSH host key and install the root-managed identity
mapping before enabling the Broker:

```bash
install -d -o root -g safeterm-broker -m 0750 /etc/safeterm/access/ssh
ssh-keygen -q -t ed25519 -N '' -f /etc/safeterm/access/ssh/ssh_host_ed25519_key
chown root:safeterm-broker /etc/safeterm/access/ssh/ssh_host_ed25519_key
chmod 0640 /etc/safeterm/access/ssh/ssh_host_ed25519_key
install -o root -g safeterm-broker -m 0640 deploy/ssh/users.example.json /etc/safeterm/access/ssh/users.json
```

Replace the example password hash before startup, or configure an optional public key.
See [SSH authentication](../../docs/ssh-authentication.md). The identity file maps each credential
and matching SSH username to roles that exist in the signed catalog. A key may
belong to only one user. Connect with `ssh -t -p 2222 alice@HOST`; the PTY runs
the SafeTerm command menu and never starts an operating-system shell.

Install the Console credential file as root:safeterm-identity mode 0640, with
root-owned parent directories. The shared safeterm account must not join the
safeterm-identity group. The automated installer installs a disabled example,
creates the one restricted Linux login and starts the isolated Identity service.
Provision operators with `sudo termctl user add console --id alice --roles viewer`;
no per-person Linux accounts are needed. Enter using `sudo -iu safeterm`, then
supply the internal username/password. Physical Linux login may additionally
use `passwd safeterm` to unlock the shared entry account. PAM is unchanged.
See [Console authentication](../../docs/console-authentication.md).

Before deployment, calculate SHA-256 for the exact target-host binaries,
update every executable registry entry, and sign the complete catalog with the
deployment's offline Ed25519 private key. Gateway and every Runner fail startup
when the signature is absent or invalid. Runners also verify canonical path,
file mode, native ELF type, and SHA-256 immediately before each process, then
execute the held `/proc/self/fd` descriptor. Keep executable mounts read-only;
the supplied Runner units enforce this with `ProtectSystem=strict`.

Every local IPC server rejects frames above 1 MiB, applies a read or operation
deadline, and uses fixed connection admission. Gateway/Runner transport permits
at most 128 concurrent connections; Audit and each Config Manager permit 64.
Connections above those ceilings are closed before a handler goroutine is
created. These application limits complement the finite `MemoryMax` and
`TasksMax` values in the units.

The service-manager accepts only argument-free unit/action pairs from the
signed catalog. The installer generates
`/etc/sudoers.d/safeterm-service-manager` with every exact permitted systemctl
argv and validates it with `visudo`; it never emits a wildcard. Status-only
operations belong to `service-viewer`, while lifecycle changes require the
non-default `service-operator` role. The service-manager unit permits the sudo
set-user-ID transition but retains its other sandbox and resource controls.

The log-reader Runner has no Linux capability and executes only signed process
operations. Its `log` operation maps public aliases to fixed catalog paths and
uses `tail -F`; callers can never provide a path. The Ubuntu `adm` group grants
read access to selected logs. Custom service logs and their logrotate policy
must retain group `adm` and mode 0640 (or use a deliberately narrower site
group and matching unit override).

M11 also enables bounded long-running sessions over the authenticated Unix
socket. The network-observer unit permits at most 64 sessions, 16 KiB chunks,
1 MiB combined output per session, a five-minute idle interval, and a 30-minute
execution deadline. A semantic terminate first interrupts the dedicated process
group, waits two seconds, sends terminate, waits another two seconds, and uses a
forced kill only when necessary. Disconnection, timeout, output exhaustion, and
backpressure all fail closed. Tune these values deliberately per deployment;
never remove the bounds.

The Broker accepts one interactive session channel per connection and rejects
exec/subsystems/forwarding/environment injection. SSH and Console share the
contextual terminal: ?, execute, config, show, whoami, clear and exit. Tab and
question-mark help use authenticated authorization views. Ctrl+C stops streams;
Ctrl+] exits. See [the terminal guide](../../docs/restricted-terminal.md).

All Config Managers read the root-owned generated registry at
/var/lib/safeterm/generated/resources.json. They have no writable application
mounts and need no legacy /etc/safeterm-managed directories. Empty profiles are
valid. File updates go exclusively through the restricted root writer, whose
sandbox is generated from administrator definitions and preserves application
ownership, ACLs, UTF-8 BOM and INI comments. Do not create/chown dummy directories.
Fresh installations do not create `/etc/safeterm-managed`. Successful updates
remove its empty legacy profile directories. Directories containing data or
symlinks remain intact and are reported for administrator review.

Use resources.d for configuration files and site JSON directories for roles,
operations, executable approvals, policies and Runner profiles. Apply provisions
custom locked Runner identities and generated units, updates Gateway dependencies,
restarts consumers in dependency order and scopes Audit and sudo by profile.
Never edit generated catalog/unit files directly. See [site configuration](../../docs/site-configuration.md)
for schemas, ceilings, recovery and lifecycle behavior.
