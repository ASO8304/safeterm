# SafeTerm Deployment and End-to-End Demo

For a package that is built on a connected machine and installed without Git,
Go, or Internet access on the Ubuntu target, follow the
[online and air-gapped installation guide](offline-installation.md).

## Fresh-host scope

M15 targets a fresh, disposable Linux host with systemd. Run the full demo in
a VM, not on a workstation or an existing production host. The installer
creates system users, writes `/etc/safeterm`, installs units, and enables nine
services. In-place upgrades and uninstall are outside the M15 contract.

## Prerequisites

- Linux with systemd and root access
- Go 1.26 or newer on the bundle-building host
- `ssh`, `ssh-keygen`, `ssh-keyscan`, `sha256sum`, and standard account tools
- `/usr/bin/uname`, `/usr/bin/tcpdump`, and `/usr/bin/systemctl`
- TCP port 2222 available for the SSH Broker

## Production bundle workflow

On the target host, create an unsigned catalog containing hashes of that exact
host's executables:

```bash
go run ./cmd/catalog-prepare --source ./configs --output /tmp/safeterm-catalog
```

Transfer `/tmp/safeterm-catalog` to the offline signing system. Generate or use
an existing protected Ed25519 key and return only the public key and signature:

```bash
catalog-sign -generate-key \
  -private-key /offline/safeterm-catalog.key \
  -public-key /tmp/safeterm-catalog/trust/catalog-ed25519.pub

catalog-sign -config /tmp/safeterm-catalog \
  -private-key /offline/safeterm-catalog.key \
  -signature /tmp/safeterm-catalog/catalog.sig
```

Add the root-managed SSH host private key at
`ssh/ssh_host_ed25519_key`, the public-key identity map at `ssh/users.json`,
the local-login identity map at `console/users.json`, and the resource registry
at `config-manager/resources.json`. A production console map can start from
`deploy/console/users.example.json`; keep only the roles that operator needs.
Prepare a managed
seed directory whose layout is `PROFILE/basename(registered path)`.

Build the bundle:

```bash
deploy/scripts/build-bundle.sh \
  --config /tmp/safeterm-catalog \
  --managed /secure/staging/managed \
  --output /tmp/safeterm-bundle
```

The builder runs semantic preflight and writes `SHA256SUMS`. Copy the complete
directory without modifying it. Do not include the private catalog key or any
SSH client private key. Bundle preflight scans the staged runtime configuration
and rejects private-key material outside the required SSH host-key path.

On the fresh runtime host, inspect the plan before making changes:

```bash
sudo deploy/scripts/install.sh --bundle /tmp/safeterm-bundle --dry-run
```

Then install and start the services:

```bash
sudo deploy/scripts/install.sh --bundle /tmp/safeterm-bundle
sudo /usr/local/bin/safeterm-preflight --installed
```

Connect with an authorized key:

```bash
ssh -t -p 2222 alice@HOST
```

Inside the restricted terminal, run `whoami`, `list`, `run system_info`, and
`config list`. Long-running `trace` output can be stopped with Ctrl+C, which
maps to the semantic terminate protocol.

The prompt includes the authenticated identity, for example
`safeterm::alice> `. Tab completes only authorized commands, operations,
parameters, config resources, sections, and fields. Arrow keys provide cursor
movement and session history; Home, End, Delete, Backspace, Ctrl+A, Ctrl+E,
Ctrl+U, Ctrl+W, and Ctrl+L provide normal line editing.

For local VM or physical-console access, enroll a password after installation:

```bash
sudo passwd safeterm
```

Then log in as `safeterm` from the Linux console. The account's fixed login
shell is `/usr/local/bin/safeterm-console`, so it displays
`safeterm::safeterm> ` and the same restricted menu. The installer intentionally
does not choose a password. Roles are controlled only by the root-owned
`/etc/safeterm/console/users.json`; the login user cannot supply roles, paths,
or a replacement identity file. After changing that file, start a new login
session. No SafeTerm service restart is required.

## Disposable-host demo

The demo generates temporary signing, host, and client keys, refreshes target
digests, builds a bundle, installs the services, pins the Broker host key, and
runs a scripted restricted-terminal session:

```bash
sudo deploy/demo/run-demo.sh
```

Temporary private keys are deleted when the script exits. The installed demo
contains the sample secret `demo-token-change-me`; never reuse the demo bundle
or its identities in another environment.

## Verification and troubleshooting

```bash
systemctl --no-pager --full status 'safeterm-*'
journalctl -u safeterm-audit -u safeterm-gateway -u safeterm-ssh-broker
sudo /usr/local/bin/safeterm-preflight --installed
sudo /usr/local/bin/safeterm-audit --help
```

If preflight reports an executable digest mismatch, repeat `catalog-prepare`
on the target host and obtain a new offline signature. Do not edit a signed
catalog or signature in place. If a service fails, inspect its journal and fix
the owning file, account, group, capability, or catalog boundary; do not add the
service to broader groups or grant a general shell. The Audit store remains at
`/var/lib/safeterm/audit.jsonl` and must not be copied into public bug reports.
