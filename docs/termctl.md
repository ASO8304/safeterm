# termctl administration

Run termctl from an ordinary Linux administrator shell. Mutations require sudo.
The restricted SSH/Console terminal has its own grammar and never grants an OS shell.

## Define resources

```sh
sudo cp /etc/safeterm/resources.d/TEMPLATE.json /etc/safeterm/resources.d/myapp.json
sudo nano /etc/safeterm/resources.d/myapp.json
sudo termctl check
sudo termctl apply
```

See [resource definitions](resource-definitions.md) for independent config/log/service
blocks, roles, path restrictions, signing and portability. TEMPLATE.json is always
ignored. Only current JSON sources and verified package built-ins are compiled;
old catalog/config activation commands and source layouts are removed (ADR-032).

Advanced activation options:

```sh
sudo termctl apply --export /root/unsigned-candidate
sudo termctl apply --signed-catalog /root/signed-candidate
sudo termctl apply --key-file /secure/catalog.key
sudo termctl apply --rollback
sudo termctl apply --recover
```

Export writes an unsigned candidate into a new root-controlled directory. Sign
with the trust key using safeterm-catalog-sign, then return the matching signed
candidate. Rollback restores the previous complete activation; recover handles an
interrupted one. Neither rewrites source JSON or application values. Unchanged
apply does not restart consumers. Changed activation disconnects active sessions.

## Inspect and manage

```sh
termctl catalog list
termctl catalog describe ping
termctl role list
termctl role describe netops
termctl config list
termctl config describe myapp
termctl service status all
sudo termctl service restart all
sudo termctl service start ssh
sudo termctl service stop ssh
sudo termctl service enable ssh
sudo termctl service disable ssh
```

Service names select a fixed SafeTerm unit map. Arbitrary units/commands are
refused. External application services use resource definitions and the restricted
terminal's `execute service ACTION RESOURCE`. Only exact approved actions receive
the dedicated service runner's restricted grants, never general sudo.

## Users

```sh
sudo termctl user add console --id alice --roles viewer,netops,config-viewer,config-operator
sudo termctl user add ssh --id alice --roles viewer,netops
sudo termctl user password console --id alice
sudo termctl user password ssh --id alice
sudo termctl user auth console --id alice --roles viewer
sudo termctl user del ssh --id alice
sudo termctl user list
sudo termctl service restart ssh
sudo -iu safeterm
```

Add prompts twice without echo and stores bcrypt hashes only. Passwords require
8–72 printable ASCII characters with uppercase, lowercase and a number. For
unattended provisioning use --password-hash-file FILE; an optional SSH key uses
--authorized-key-file FILE. Console and SSH maps are independent. Console users
share the restricted Linux entry account and authenticate internally; no per-person
Linux account or PAM alteration is needed. Console changes take effect immediately;
SSH identity changes require a broker restart. The installer creates its initial
internal identity in both maps. No user gains root from a SafeTerm role.

## Packages and completion

```sh
sudo termctl update /path/to/extracted/package
sudo termctl rollback
sudo termctl uninstall
source /usr/share/bash-completion/completions/termctl
```

New-layout updates preserve source JSON and identities and rebuild derived data.
Old-layout updates are refused; no automatic migration/deletion is performed.
Package rollback restores the latest installer snapshot, including accounts and
state, so use it before unrelated account changes. Uninstall retains data and
snapshots. See [installation](offline-installation.md) for signing and recovery.
The restricted editor has independent authorization-aware Tab and `?` Enter help.
`execute trace ` completes `host=`, `port=` and `protocol=` without subcommands.
At least one of host or port must be specified; protocol is optional. Protocol
values complete after `protocol=`. The value `any` is rejected for all three inputs.
`execute ping ` completes `host=` and `count=`; only host is required, and
omitting count sends 1000 requests (also the maximum).
Ctrl+C stops a stream; Ctrl+] exits the whole session.

## Default definitions and site overrides

See [site configuration](site-configuration.md) for roles.d, operations.d,
executables.d, profiles.d and policies.d, complete schemas, precedence and safety
bounds. `termctl defaults TYPE NAME` prints an editable packaged default.
`termctl profile list/describe`, `termctl executable list/describe`, and
`termctl executable hash PATH` provide discovery and digest inspection. All new
sources share check/apply/recovery and shell completion. `termctl service ...`
also accepts site-NAME for a provisioned Runner.
