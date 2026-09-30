# Linux systemd deployment

M9 ships an independent append-only Audit Service alongside one unprivileged
Node Gateway and three capability-isolated Runner units. All Runner units use
the same `safeterm-runner` binary, but they run as
different users with separate runtime directories, sockets, catalogs, resource
limits, and capability bounding sets.

Required service accounts:

```text
safeterm-gateway
safeterm-broker
safeterm-diagnostics
safeterm-network
safeterm-service-manager
safeterm-audit
```

The Runner accounts use `safeterm-gateway` as their service group so the
Gateway can reach their socket directories. Runner servers still verify the
Gateway UID with `SO_PEERCRED`; group membership alone cannot authorize a
request. The Gateway similarly verifies each Runner UID.

Gateway and Runner accounts also belong to the `safeterm-audit` group so they
can reach the Audit Service socket. The Audit Service independently verifies
each producer UID with `SO_PEERCRED`, runs without execution capabilities, and
writes only to `/var/lib/safeterm/audit.jsonl`. Audit records contain argument
names but never raw argument values. Every append is synchronized before the
producer receives acknowledgement, and each record is linked to the previous
record by SHA-256 to detect modification, reordering, and deletion inside the
retained chain. The hash chain is corruption evidence, not a digital signature
or an external checkpoint against complete-file replacement or tail truncation.

Install the binaries as `/usr/local/bin/safeterm-gateway`,
`/usr/local/bin/safeterm-runner`, and `/usr/local/bin/safeterm-audit`; install the catalog read-only at
`/etc/safeterm`, and copy the unit files to `/etc/systemd/system`.

The shipped service-manager catalog contains no active operation and its unit
has no Linux capability. Before adding service operations, deployment tooling
must install an OS policy that grants `safeterm-service-manager` only the exact
systemd units and actions present in the approved catalog. Never grant this
account unrestricted `sudo` or general systemd management authority.

M10 adds executable hash and canonical-path verification. M12 adds the remote
Broker, authenticated identity, and catalog proposal/approval workflow.
