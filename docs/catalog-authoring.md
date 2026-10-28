> Historical workflow, superseded by ADR-032. Its write commands and source
> paths are removed. Use [resource definitions](resource-definitions.md) and
> [termctl](termctl.md) for the supported interface.

# Catalog Authoring Guide

SafeTerm exposes logical operations, never raw commands. The commented
`TEMPLATE.jsonc` files beside each catalog directory document every supported
field. They use JSON with comments for humans and are ignored by the runtime,
which loads only `*.json` files.

Identity templates live in `deploy/ssh/users.template.jsonc` and
`deploy/console/users.template.jsonc`. Managed configuration files have a
complete schema example in `deploy/config-manager/resources.template.jsonc`.
Offline packages place these three deployment templates under
`payload/templates/`; catalog templates remain under `payload/catalog-source/`.

## Add an allowlisted log

The shipped `log` operation maps a public alias to a catalog-owned path:

```json
"value_map": {
  "service_A": "/var/log/safeterm-readable/service_A.log",
  "system": "/var/log/syslog"
}
```

Users run either form:

```text
execute log service_A
execute log source=service_A
```

They cannot submit a path. Validation accepts only a `value_map` key and
translation substitutes its fixed value into `tail -n 50 -F -- PATH`. The
operation streams through the log-reader Runner and Ctrl+C sends semantic
termination.

After adding aliases on an installed host, use `sudo termctl catalog apply
--dry-run`, then `sudo termctl catalog apply` to validate, sign and activate them.
See [catalog activation](catalog-activation.md) for offline signing and recovery.

For another service, add its alias/path to the signed catalog and grant the
dedicated reader account access without making the file world-readable:

```bash
sudo chgrp adm /var/log/example/service.log
sudo chmod 0640 /var/log/example/service.log
```

Configure logrotate to retain group `adm` and group-read mode after rotation.
The log-reader service belongs to `adm`, has no Linux capabilities, and can
execute only the signed `tail` operation. A narrower deployment may replace
`adm` with a dedicated site group in the unit and file rotation policy.

## Add bounded network diagnostics

The shipped `ping` operation sends bounded IPv4 echo requests with a
two-second per-packet timeout. Only host is required. Count defaults to 1000
(the maximum); `count=VALUE` accepts 1–1000. Tab offers only `host=` and `count=`
and stops offering parameters already supplied. The `-c` alias remains accepted
when explicitly typed. Only the validated host value is accepted:

```text
execute ping host=192.168.45.140 count=5
```

Grant `ping` through the `netops` policy. Keep the executable in the
`network-observer` profile so its `CAP_NET_RAW` boundary is separate from
diagnostic and service-manager processes.

The catalog exposes one `capture` operation with optional typed `host`, `port`
and `protocol` parameters. `required_any: ["host", "port"]` rejects empty
requests and protocol-only requests with `host or port must be specified`.
`any` is not accepted for host, port or protocol. Protocol choices are `tcp`,
`udp`, `icmp`, `arp` and `ip`. Tab completes names and protocol values.

```text
execute capture host=192.0.2.1
execute capture port=999
execute capture host=192.0.2.1 port=999 protocol=tcp
```

The signed `argument_groups` each have a `when` parameter and an `args_template`.
A group is included only when its parameter has a nonempty effective value
(including a configured default). Included groups are joined with the literal
`argument_group_separator: ["and"]`; there is no leading/trailing separator.
All placeholders remain whole argv tokens and are validated before execution.
No caller-built filter expressions or shell are accepted. Protocol predicates
are standalone BPF tokens, so `arp` is emitted as `arp`, rather than `proto arp`.


## Add typed systemd control

Create one argument-free operation for every approved unit/action pair:

```json
{
  "name": "restart_example",
  "description": "Restart the approved example service.",
  "runner_profile": "service-manager",
  "executor_kind": "systemd",
  "systemd": {
    "unit": "example.service",
    "action": "restart"
  },
  "parameters": [],
  "long_running": false
}
```

Supported actions are `start`, `stop`, `restart`, `reload`, `enable`,
`disable`, and `status`. Never accept the unit or action as a user parameter.
The installer derives `/etc/sudoers.d/safeterm-service-manager` from the signed
typed operations, validates it with `visudo`, and grants no wildcard command.

Assign status operations to `service-viewer`. Assign mutating operations only
to `service-operator`; that role is not included in fresh default identities.
To grant it to an existing SSH identity, edit the root-owned
`/etc/safeterm/access/ssh/users.json`, add `service-operator`, and restart the Broker
from the host administration console.

## Publish catalog changes

1. Edit `configs/catalog.json` and the relevant JSON arrays. Increment the
   catalog version.
2. Run `make verify` and build a new offline package.
3. On the exact target, run `./install.sh --prepare-catalog DIR` so every
   executable receives its target-host SHA-256.
4. Sign the prepared directory on the protected signing host.
5. Return the signed directory and run `sudo ./install.sh --signed-catalog DIR`.
6. Run `safeterm-preflight --installed`, then use `list`, `describe NAME`, and
   the operation from the restricted terminal.

For catalog-only changes on an installed host, prefer `termctl catalog apply`
rather than rebuilding a package. It keeps its own verified rollback checkpoint.
Do not restart services after unsigned live edits; apply them first, or work on a
copy with `--source DIR`. Package/unit changes still use the installer workflow.

## Streaming tcpdump output

The shipped `capture` operation uses `-l -nn`. SafeTerm captures stdout through a pipe;
`-l` makes tcpdump flush each text line instead of retaining a block until
termination. `-nn` disables hostname and port-name resolution so lookup latency
cannot delay packet rendering. The fixed capture/filter arguments remain
catalog-owned. Runtime streaming cannot flush bytes still buffered inside the
child process.

On a test host, start `execute capture port=999`, generate TCP traffic to
port 999 from another terminal, and verify packet lines appear before Ctrl+C.
Termination should still return a final result and record durable completion
without reusing the canceled execution context. Runner completion auditing has
an independent five-second deadline and still fails closed if audit fails.

Changing these arguments changes the signed catalog. Use the normal package
upgrade/signing procedure; editing the active JSON without signing is not valid.

## Ping sessions

Ping uses bounded streaming with a default and maximum count of 1000.
Capture and ping support Ctrl+C to stop the operation and Ctrl+] to close the
session. IPC waits are closed on context cancellation, including stream startup.
The packaged nslookup and traceroute operations are no longer provided.
