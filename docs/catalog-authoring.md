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
run log service_A
run log source=service_A
```

They cannot submit a path. Validation accepts only a `value_map` key and
translation substitutes its fixed value into `tail -n 50 -F -- PATH`. The
operation streams through the log-reader Runner and Ctrl+C sends semantic
termination.

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

The shipped `ping` operation is intentionally fixed to three IPv4 echo
requests with a two-second per-packet timeout. Only the validated host value
is accepted, and it is passed as one argument to `/usr/bin/ping`:

```text
run ping host=192.168.45.140
```

Grant `ping` through the `netops` policy. Keep the executable in the
`network-observer` profile so its `CAP_NET_RAW` boundary is separate from
diagnostic and service-manager processes.

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
`/etc/safeterm/ssh/users.json`, add `service-operator`, and restart the Broker
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

The installer creates a rollback transaction before activation. Do not edit
the active `/etc/safeterm` catalog directly: its detached signature will no
longer match and services fail closed on restart.
