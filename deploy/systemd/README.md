# Linux systemd deployment

SafeTerm ships authenticated restricted SSH and local-console Brokers, three isolated Config
Managers, and an independent append-only Audit Service alongside one unprivileged
Node Gateway and four capability-isolated Runner units. All Runner units use
the same `safeterm-runner` binary, but they run as
different users with separate runtime directories, sockets, catalogs, resource
limits, and capability bounding sets.

Use the automated bundle, preflight, installer, and demo procedures in the
[deployment guide](../../docs/deployment.md). The remainder of this document
describes the service identities and security boundaries enforced by the units.

Required service accounts:

```text
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
`broker_type` (`ssh` or `console`), while each
Config Manager UID may use unary routing only for the typed activation and
health operations assigned to its own profile.

Gateway and Runner accounts also belong to the `safeterm-audit` group so they
can reach the Audit Service socket. The Audit Service independently verifies
each producer UID with `SO_PEERCRED`, runs without execution capabilities, and
writes only to `/var/lib/safeterm/audit.jsonl`. Audit records contain argument
names but never raw argument values. Every append is synchronized before the
producer receives acknowledgement, and each record is linked to the previous
record by SHA-256 to detect modification, reordering, and deletion inside the
retained chain. The hash chain is corruption evidence, not a digital signature
or an external checkpoint against complete-file replacement or tail truncation.

Install the binaries as `/usr/local/bin/safeterm-ssh-broker`, `/usr/local/bin/safeterm-console`, `/usr/local/bin/safeterm-config-manager`, `/usr/local/bin/safeterm-gateway`,
`/usr/local/bin/safeterm-runner`, `/usr/local/bin/safeterm-audit`, and keep
`catalog-sign` only on an offline administration host. Install the catalog,
`catalog.sig`, and the separately provisioned public key at
`/etc/safeterm/trust/catalog-ed25519.pub` read-only, then copy the unit files to
`/etc/systemd/system`.

Generate a dedicated Ed25519 SSH host key and install the root-managed identity
mapping before enabling the Broker:

```bash
install -d -o root -g safeterm-broker -m 0750 /etc/safeterm/ssh
ssh-keygen -q -t ed25519 -N '' -f /etc/safeterm/ssh/ssh_host_ed25519_key
chown root:safeterm-broker /etc/safeterm/ssh/ssh_host_ed25519_key
chmod 0640 /etc/safeterm/ssh/ssh_host_ed25519_key
install -o root -g safeterm-broker -m 0640 deploy/ssh/users.example.json /etc/safeterm/ssh/users.json
```

Replace the example public key before startup. The identity file maps each key
and matching SSH username to roles that exist in the signed catalog. A key may
belong to only one user. Connect with `ssh -t -p 2222 alice@HOST`; the PTY runs
the SafeTerm command menu and never starts an operating-system shell.

Install `deploy/console/users.example.json` as the root-owned
`/etc/safeterm/console/users.json`. The automated installer creates the
unprivileged `safeterm` login account, assigns `/usr/local/bin/safeterm-console`
as its shell, and grants only the socket traversal groups needed by that client.
Set its password explicitly with `passwd safeterm` before console login. The
console executable accepts no path or role overrides and still depends on
Gateway and the isolated Config Managers and Runners.

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

The Broker accepts only one session channel per connection and rejects SSH
exec, subsystems, forwarding, agent forwarding, and environment changes. Its
fixed shell grammar is `help`, `list`, `describe`, `run`, `whoami`, `clear`,
and `exit`, plus the structured `config` subcommands. Long-running output uses M11 streaming; Ctrl+C sends the semantic
termination request to Gateway. Tab completion is derived from authorized
catalog/config views, and history remains in memory only for the session.
Catalog proposal/approval and additional
client adapters remain separate control-plane work.

Provision the M13 registry and three dedicated writable directories before
starting the Config Managers. The registry must be owned by root and must not
be group- or world-writable. Each managed directory belongs to exactly one
Config Manager identity so atomic temporary files and bounded backups do not
broaden access to unrelated configuration:

```bash
install -d -o root -g root -m 0755 /etc/safeterm/config-manager
install -o root -g root -m 0644 deploy/config-manager/resources.example.json \
  /etc/safeterm/config-manager/resources.json
install -d -o safeterm-config-application -g safeterm-config-application -m 0700 \
  /etc/safeterm-managed/application
install -d -o safeterm-config-database -g safeterm-config-database -m 0700 \
  /etc/safeterm-managed/database
install -d -o safeterm-config-network -g safeterm-config-network -m 0700 \
  /etc/safeterm-managed/network
```

Replace the example resource paths, schemas, and roles for the target host and
create each initial file before service startup. A Config Manager refuses an
invalid document, a symlinked managed file, an unavailable validator or typed
activation operation, and any resource outside its selected profile. The SSH
Broker reaches each manager through its own authenticated Unix socket. Use
`config list`, `config describe ID`, `config show ID`, and
`config edit ID SECTION KEY` inside the restricted terminal. Use `-` for a
root-level JSON field. Protected resources return a proposal token that a
different authorized identity applies with `config apply TOKEN`; retained
backups can be restored with `config rollback ID`.

Gateway must load the same root-owned registry as the Config Managers. Its
manager mapping uses `PROFILE=USER` entries, for example
`application=safeterm-config-application`. Profiles and UIDs must be unique.
Every activation operation must also exist in the signed catalog as an
argument-free `systemd` operation; Gateway refuses startup if the registry and
catalog cannot produce a safe allowlist for a configured manager.
