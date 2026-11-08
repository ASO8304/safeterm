# termctl administration

Run termctl from an ordinary Linux administrator shell. Mutations require sudo.
The restricted SSH/Console terminal has its own grammar and never grants an OS shell.

## Check the installed version

```sh
termctl version
termctl --version
/path/to/extracted/package/payload/bin/safeterm-termctl version
```

Version output identifies that executable's embedded release/module version,
source commit, clean/modified source state, Go compiler, platform and executable
path. It requires neither root nor working configuration/services. Untagged or
unstamped builds are identified without claiming a release. Comparing the
packaged and installed commands helps detect an older extracted archive or a
package that has not yet been installed. Extracting a release does not update
`/usr/local/bin/termctl`; use `sudo termctl update PACKAGE_DIR` to install it.

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
termctl catalog describe tcp-test
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

Catalog and role inspection display `tcp-test`, matching the restricted terminal.
`catalog describe tcp-test` also identifies the legacy signed operation ID
`telnet`, retained for existing permission maps. The deprecated `telnet` alias
still resolves to that description. `termctl defaults operation tcp-test` accepts
the public name and emits an override with the original signed ID; do not rename
that ID when preserving existing permissions.

Service names select a fixed SafeTerm unit map. Arbitrary units/commands are
refused. External application services use resource definitions and the restricted
terminal's `execute service ACTION RESOURCE`. Only exact approved actions receive
the dedicated service runner's restricted grants, never general sudo.

## Users

```sh
sudo termctl user add console --os-user alice --roles viewer,netops,config-viewer,config-operator
sudo termctl user add ssh --id remote-alice --roles viewer,netops
sudo termctl user password ssh --id remote-alice
sudo termctl user auth console --id alice --os-user alice --roles viewer
sudo termctl user del ssh --id remote-alice
sudo termctl user list
sudo termctl service restart ssh
sudo -iu alice
```

`add console --os-user` creates a new Linux account with the SafeTerm login shell,
private primary group, required socket groups and root-controlled role mapping.
It prompts twice without echo, waits for the reloaded services and sets the Linux
password through the system's PAM password tool. Existing usernames, groups and
home paths are refused, never adopted or modified. `--id` defaults to `--os-user`.
SafeTerm services restart; existing sessions must reconnect. Failure leaves the
new account locked and removes its new mapping, without deleting home data.

SSH add prompts twice without echo and stores bcrypt hashes only. Passwords require
8–72 printable ASCII characters with uppercase, lowercase and a number. For
unattended provisioning use --password-hash-file FILE; an optional SSH key uses
--authorized-key-file FILE. These hash/key options apply to the separate service
credential stores, not Linux Console provisioning. Console and SSH enrollment
are separate installer steps with independent usernames and passwords.

For compatibility, `add console --id ID` without `--os-user` still creates an
internal legacy Console user for the shared `safeterm` entry account. It does not
create a Linux account. Existing internal users are preserved. No user gains
root from a SafeTerm role.

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
`execute capture ` completes `host=`, `port=` and `protocol=` without subcommands.
At least one of host or port must be specified; protocol is optional. Protocol
values complete after `protocol=`. The value `any` is rejected for all three inputs.
`execute ping ` completes `host=` and `count=`; only host is required, and
omitting count sends 1000 requests (also the maximum).
Ctrl+C stops a stream; Ctrl+] exits the whole session.

## Default definitions and site overrides

See [site configuration](site-configuration.md) for access/roles.d, commands/operations.d,
commands/executables.d, commands/profiles.d and access/policies.d, complete schemas, precedence and safety
bounds. `termctl defaults TYPE NAME` prints an editable packaged default.
`termctl profile list/describe`, `termctl executable list/describe`, and
`termctl executable hash PATH` provide discovery and digest inspection. All new
sources share check/apply/recovery and shell completion. `termctl service ...`
also accepts site-NAME for a provisioned Runner.

## Ubuntu Server 24.04 network management

Use `config network interface` and `config network route` to save candidates, then `execute network apply` and `execute network confirm` for a timed Netplan trial. The root-owned `/etc/safeterm/network/policy.json` policy starts with no editable interfaces. The `netops` role permits network work by default within the administrator interface allowlist. Existing filenames are discovered automatically. See [the network guide](network-configuration.md) for setup, examples and recovery.

System diagnostics use `execute system_info host`, `execute system_info memory` and `execute system_info disk`.
