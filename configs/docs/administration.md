# SafeTerm administrator configuration

SafeTerm includes ready-to-use, signed package defaults. No custom definitions
are needed for normal installation. Add JSON files only when changing a default
or adding a site-specific capability.

Run `termctl version` (or `termctl --version`) to see this binary's release, commit,
source state, Go compiler, platform and executable path without loading config.
Before installing, compare it with `payload/bin/safeterm-termctl version` from
the extracted package. Extracting an archive alone does not update installed
binaries. Use `sudo termctl update PACKAGE_DIR` for installation.

`termctl catalog` and role inspection show the public `tcp-test` command.
`termctl catalog describe tcp-test` explains the retained `telnet` permission ID
and deprecated alias. `termctl defaults operation tcp-test` emits the compatible
definition with that original ID for existing permission maps.

```text
/etc/safeterm/
  access/
    ssh/                 SSH users and host key
    console/             Console users
    roles.d/             Role definitions
    policies.d/          Exact role-to-operation permissions
  commands/
    operations.d/        Command definitions and arguments
    executables.d/       Approved binary paths and SHA-256 hashes
    profiles.d/          Runner isolation and resource limits
  resources.d/           Application configuration, services and logs
  network/policy.json    Editable interfaces and network roles
  trust/                 Trusted catalog public key
  docs/                  Administration, network and diagnostic guides
```

Each definition directory contains an always-inactive TEMPLATE.json and English
README.md/README.txt. Copy a template to a new *.json filename before editing.
Only root-owned, non-writable-by-others JSON files are active; duplicate/unknown
properties and invalid references are rejected. No legacy catalog-source loading
or merging of generated data is supported.

For an existing default, emit a correctly formatted override:

```sh
sudo termctl defaults operation ping | sudo tee /etc/safeterm/commands/operations.d/ping.json >/dev/null
sudo nano /etc/safeterm/commands/operations.d/ping.json
sudo termctl check
sudo termctl apply
```

Other default types: role, profile, executable, policy. Built-in values remain
unchanged when no override exists. Removing an override restores the packaged
default; removing an added definition removes it if no references remain.
access/policies.d entries are exact replacements, without implicit inherited grants.
Use termctl executable hash PATH to obtain a hash; approvals never refresh silently.
Use termctl role/profile/executable list or describe to inspect compiled values.

Runtime data is root-owned under /var/lib/safeterm/generated; never edit it.
Verified package built-ins are under /usr/share/safeterm/defaults; never edit them.
Audit owns only /var/lib/safeterm/audit. The local signer key, when selected,
remains under /root/.config/safeterm.

Profile overrides use fixed packaged isolation templates. A new profile gets a
separate locked system user and generated Runner service/socket through apply.
No arbitrary root identity, systemd text, socket or capability escalation is
accepted. Removed profiles lose their runtime grants; dormant managed accounts
are retained to avoid UID reuse. See the full site-configuration guide in the
repository docs for exact bounds, policy ordering and operational limits.

`sudo termctl apply --rollback` restores a completed definition activation;
`sudo termctl apply --recover` handles interruption. These do not rewrite source
JSON or application values. Package `termctl rollback` is separate and restores
an installation snapshot. Updates preserve custom source files and credentials;
reference templates/guides refresh and generated data/services are rebuilt.
INI comments, whitespace, UTF-8 BOM, ownership and ACLs remain preserved.

This layout extends ADR-032 through ADR-033. Old configurations under direct
operations/roles/profiles/etc directories are refused, never imported or deleted.
Copy only modern source JSON to another host and provision its actual binaries,
files, units, narrow access and signing method separately.

---

# Site configuration: ready defaults and administrator overrides

SafeTerm starts with verified packaged defaults. No site definition is required.
The JSON directories under /etc/safeterm are the only administrator definition
inputs; generated data is never used as source and old catalog directories are
not supported. This extends ADR-032 without restoring legacy source loading.

| Directory | Object in each active JSON file |
|---|---|
| resources.d | Independent service/config/log resources |
| access/roles.d | Role name and description |
| commands/operations.d | Operation definition, optional role assignment or disabled flag |
| commands/executables.d | Executable ID, exact path, approved SHA-256 and allowed profiles |
| commands/profiles.d | Runner template and bounded isolation/runtime limits |
| access/policies.d | One role's exact list of authorized operations |

Every directory has an inactive TEMPLATE.json and English README.md/README.txt.
The exact TEMPLATE.json is skipped before opening, even if invalid or a symlink.
Other *.json files are active. Rename to *.json.disabled to deactivate. Inputs
must be root-owned, single-link regular files without group/world write; all
parent directories must be root-controlled. Unknown/duplicate properties, duplicate
IDs in one directory, invalid references and oversized inputs fail validation.
Limits per directory: 256 active files, 1 MiB each, 8 MiB total; generated files
must each fit 1 MiB. Source filenames are arbitrary; object names/IDs determine
replacement. Removing a built-in override restores the packaged default. Removing
an added definition removes it, but still-referenced definitions fail closed.

## Edit an existing default

From a normal Linux administrator shell:

```sh
sudo termctl defaults operation ping | sudo tee /etc/safeterm/commands/operations.d/ping.json >/dev/null
sudo nano /etc/safeterm/commands/operations.d/ping.json
sudo termctl check
sudo termctl apply
```

The emitted object has an `operation` field containing the complete definition.
For example, edit a parameter's default count or its min/max. Omitting `roles`
retains packaged policy membership. An explicit `roles` array replaces membership
for this operation in every role; an empty array grants it to nobody. Setting
`disabled: true` removes the operation and its policy references entirely. Names
starting resource_ are reserved for resources.d-generated operations. Arguments
remain argv tokens and whole-token placeholders; no shell strings are constructed.

Use `termctl defaults role viewer`, `termctl defaults profile diagnostics`,
`termctl defaults executable uname`, or `termctl defaults policy viewer` for other
source shapes. These inspect signed package defaults, never active site edits.
`termctl catalog list/describe`, `termctl role list/describe`, `termctl profile
list/describe` and `termctl executable list/describe` inspect compiled definitions.

## Add a role and operation

access/roles.d/site-reader.json:

```json
{"name":"site-reader","description":"Read site system information"}
```

commands/operations.d/site-system.json:

```json
{
  "operation": {
    "name": "site_system",
    "description": "Site system information",
    "runner_profile": "diagnostics",
    "executor_kind": "process",
    "executable_ref": "uname",
    "args_template": ["-s"],
    "parameters": []
  },
  "roles": ["site-reader"]
}
```

After check/apply, assign the role using termctl user auth/add with --roles.
Role assignment never creates a role implicitly. Existing identity roles must
continue to exist; activation does not change credentials or user role lists.
Remove a role from identities before rolling back to a generation without it.

## Policies

access/policies.d/site-reader.json can explicitly replace a role's entire operation list:

```json
{"role":"site-reader","operations":["system_info","site_system"]}
```

There are no wildcard grants or role inheritance. An empty operation list is
valid. Resource config read/edit permissions remain in resources.d and are
independent of operation policies. Compilation order is defaults, role/profile/
executable/operation overrides, generated resources, then exact policy replacements.
Thus a access/policies.d entry is authoritative over operation-level/generated grants
for that role; list every operation it should retain. The final model must pass
all cross-reference, translation, integrity and identity validation.

## Executables

operations select executable IDs. commands/executables.d defines their actual paths,
SHA-256 approvals and allowed profiles. Use:

```sh
termctl executable hash /usr/bin/uptime
```

Put that digest into a definition based on commands/executables.d/TEMPLATE.json. Its zero
digest is only an inactive placeholder. Hashing does not activate anything or
refresh stored approvals. Changed executable bytes require explicit source hash
updates and check/apply. Scripts and non-native/writable executables remain
rejected by the existing execution integrity boundary. Ordinary package updates
never silently rewrite administrator-approved executable hashes.

## Independent Runner profiles

commands/profiles.d/site-diagnostics.json:

```json
{
  "name": "site-diagnostics",
  "base": "diagnostics",
  "memory_limit_mib": 128,
  "process_limit": 32,
  "timeout_seconds": 30,
  "output_limit_bytes": 1048576,
  "max_sessions": 16,
  "idle_timeout_seconds": 300
}
```

Set an operation's runner_profile to site-diagnostics. At most 16 custom profiles
are active. Their names are 1..20 lowercase letters/digits/hyphens, starting with
a letter. Each gets a distinct password-locked, nologin system UID named
sft-site-NAME, /run/safeterm-site-NAME/runner.sock, and a generated
safeterm-runner-site-NAME.service. termctl apply provisions them; administrators
must not create those users, sockets or units manually. Reserved-name collisions
are rejected. `termctl service status/restart site-NAME` and `all` include them.
Gateway dependencies start them on boot; Audit loads their exact signed UID/profile
bindings. Every profile remains a separate authenticated IPC and process boundary.

Bases are diagnostics, network-observer, log-reader, service-manager. The base
fixes executor kind, identity/socket naming, filesystem sandbox and available
privileges. Process templates use NoNewPrivileges and no general sudo. Only
network-observer permits CAP_NET_RAW; capabilities can be reduced, never expanded.
log-reader inherits the existing adm read group. service-manager has the fixed
sudo/systemctl controller and per-profile exact unit/action grants: it cannot
use another Runner's grants. No arbitrary systemd text, root identity, custom
socket path, shell launcher, writable mount or unrestricted capability is accepted.

Optional limits inherit defaults: memory/process limits from the base, timeout
30 seconds (1800 for network/log), 1 MiB output, 64 concurrent streams and a
300-second idle timeout. Bounds: memory 32..4096 MiB, processes 4..256, timeout
1..86400 seconds, output 1024..16777216 bytes, streams 1..64, idle 1..3600 seconds.
Omitted/zero numeric overrides inherit the default. Built-in profile names may
also be overridden, with their own base and immutable identities/sockets.

Custom profiles initially inherit the base's hashed executable allowlist. An
commands/executables.d replacement is exact and must retain all intended profile members.

## Activation, rollback, update and signing

All inputs use one `termctl check/apply` workflow. Check validates without creating
users, units or restarting services. A signed, validated apply journals the old
catalog and metadata, stops consumers before Audit, publishes definitions and exact
sudoers, reconciles generated units/accounts, then restarts dependencies and
previously active consumers. A profile used by Gateway is pulled in as a dependency.
Only currently configured custom profiles are restarted; removed profiles stop
and lose their unit, Gateway dependency, root grants and Audit producer binding.

Use `sudo termctl apply --recover` after interruption and `sudo termctl apply
--rollback` for the previous completed activation. Units and scoped permissions
are regenerated from the signed checkpoint. Rollback does not rewrite source JSON
or application values; revert source files before applying again. Dormant managed
accounts and ownership markers are retained deliberately to avoid UID reuse. They
remain password-locked and have no shell/home; active services or Audit grants are
not retained. Conflicting externally modified accounts fail validation. A managed custom name
retains its original base template and UID; use a new profile name for another
base. Deleting/recreating managed accounts outside SafeTerm is rejected.

Local signing requires the existing root-private signing key. Offline signing is
`termctl apply --export DIR`, sign the exact compiled candidate on the protected
signer, then `termctl apply --signed-catalog DIR`; root provisioning happens only
after exact signature/candidate validation. Packaged defaults must verify against
the installation's existing public trust. There is no implicit trust rotation.

New-format package updates preserve every custom source directory and identity,
then regenerate configuration and Runner deployment. Package rollback/uninstall
stop and remove generated site units and Gateway dependencies. Installer snapshots
cover pre-existing site units, account databases and generated/source state.
Existing custom-role credentials are validated during package staging against
both the incoming defaults and a prior catalog signed by the same installed trust.
Only role existence is consulted: no previous operation, profile or permission is
imported. Before services start, credentials are validated strictly against the
newly compiled, signed generation. This permits updates without temporarily
rewriting or weakening user role assignments.

For unchanged offline deployments, an already trusted prior signature may be
reused only when it exactly matches the newly compiled candidate; changed
candidates still require signing. Application files and ownership are not overwritten. Portable deployment
copies only custom source JSON, provisions actual binaries/files/services and
permissions, and uses the target host's approved hashes/signing method.

## Where the default definitions live

/etc/safeterm/*.d contains administrator inputs, inactive TEMPLATE.json files and
guides. It intentionally does not contain active copies of every built-in default.
Packaged reference definitions live under /usr/share/safeterm/defaults:
operations/, roles/, profiles/, executables/ and policies/. Do not edit that
versioned reference directory or /var/lib/safeterm/generated runtime output.

Inspect defaults in administrator-editable source format:

    sudo termctl defaults operation
    sudo termctl defaults operation ping
    sudo termctl defaults role
    sudo termctl defaults profile
    sudo termctl defaults executable
    sudo termctl defaults policy

To override a packaged operation, export one definition, edit it and activate:

    sudo termctl defaults operation ping | sudo tee /etc/safeterm/commands/operations.d/ping.json >/dev/null
    sudo nano /etc/safeterm/commands/operations.d/ping.json
    sudo termctl check
    sudo termctl apply

An exact TEMPLATE.json is always inactive; other *.json files are active inputs.
Removing an override restores the packaged default on the next successful apply.
Package updates preserve administrator inputs while supplying new built-in defaults.

If a log check reports an internal unsupported-executable error, it is not proof
of a file permission failure. Current termctl permits only the exact fixed
safeterm-log-reader read probe through its catalog command helper. Real probe
failures identify the log ID, path, identity and underlying command result.
Do not change log ownership in response to an internal helper failure.

## Logical configuration paths

See resources.d/README.txt for config.contexts. Register a physical file once,
then define logical paths and case-sensitive field names such as config tunnel /
set Value_1 1400 mapped to [Tunnel].TCPMSS. Contexts and field validation are
compiled together; no Go change is needed for supported types/formats.
Config saves never activate services; remove config.on_change and use execute
service separately. The root show command is removed; show inside configuration
contexts remains available. Capture replaces trace; packaged nslookup/traceroute
operations and their installation dependencies have been removed. Administrator
policies/overrides referencing removed operation IDs must be revised explicitly.

## Ubuntu Server 24.04 network management

Use `config network interface` and `config network route` to save candidates, then `execute network apply` and `execute network confirm` for a timed Netplan trial. The root-owned `/etc/safeterm/network/policy.json` policy starts with no editable interfaces. The `netops` role permits network work by default within the administrator interface allowlist. Existing filenames are discovered automatically. See `/etc/safeterm/docs/network.txt` (also shipped as `docs/network.md`) for setup, examples and recovery.

System diagnostics use `execute system_info host`, `execute system_info memory` and `execute system_info disk`.

Service targets are registered exclusively by administrator `resources.d` definitions in a fresh installation. Packaged service roles and the isolated service-manager remain available, but have no default service operations or sudo grants. Manage SafeTerm itself from the Linux administrator shell with `sudo termctl service`. Existing explicit `commands/operations.d`/`access/policies.d` overrides remain administrator-owned and must be reviewed if they refer to removed packaged service operation IDs.

Network diagnostics use `execute ping IP [-c COUNT]` (default/max 1000) and `execute tcp-test IP [PORT]` (TCP connection test, default port 23, timeout 5 seconds). Ctrl+C cancels the operation; Ctrl+] exits SafeTerm. TCP uses netcat without application data or an interactive login. `execute udp-test IP PORT` uses netcat UDP probes; silence is inconclusive. See `docs/network-diagnostics.txt` for full details.

## Navigation and help

Login prints the main command menu once. Entering, selecting or leaving a context
is quiet. Use ? (or COMMAND ?) explicitly for help; help keeps the input prefix.
Tab in a configuration menu lists only its child names, such as interface and
route. At an editor item list it offers item names or route IDs. Inside a selected
item, use set followed by Tab for field names; empty Tab does not dump commands.

exit moves up one level and asks before discarding unsaved edits to the item being left, naming
it if changes were discarded. Other staged items remain. At an item list, exit
asks before leaving that editor and discarding its remaining draft. At the root prompt only,
exit closes the session. Ctrl+] closes the whole session from any level.
The former back command is removed.

next leaves a selected item while keeping edits staged; save validates and
commits the editor. Network save stores a candidate for the separate timed
execute network apply flow. cancel discards the current editor and returns to
root. Parent-menu exit discards retained logical drafts under that menu.
passwd changes your own login password at root; clear remains available in all contexts.

Choose a menu child by typing its name, or use edit with a relative path. From
the config menu, edit network interface opens the interface list directly.
Navigation never saves a file, applies networking or prints an automatic menu.


## Visible drafts and editor commands

Use save to validate and commit the current editor and cancel to discard it.
These replace end and abort. Network save stores a candidate; execute network
apply starts the separately confirmed trial. A * after the prompt context marks
unsaved edits. Local diff uses - and + lines and hides secrets. Interface show
includes STATE at editor entry, clear editing permissions and staged values.

exit moves up and asks before discarding unsaved edits to the item; next moves up and keeps them.
Logical contexts retain drafts in their parent menu; reopen the context to
continue or save. Parent-menu exit discards drafts under that menu. Each logical
context saves independently with its original version check.

exit moves up one level; root exit closes the session. passwd is root-only,
and clear works everywhere. Leaving a dirty item with exit asks for confirmation before discarding its edits. execute tcp-test IP [PORT] is the port
probe; execute telnet remains an alias and prints a migration notice.

## Terminal colors

Interactive SSH and Console use cyan commands/headings, a green username, a blue
configuration path, an amber unsaved * and warnings, and red errors. Interface
tables show up/allowed in green and down/read-only in amber. Diff additions are
green and removals red. Labels and symbols remain available without color.

The theme uses the terminal's ANSI palette and resets before typed input. An
unset TERM or TERM=dumb keeps plain output. Set NO_COLOR=1 in the Console or
SSH broker process environment to disable colors. SSH reads TERM from the PTY
request; arbitrary client environment variables are not accepted.

Linux-account Console login without a second prompt is described in docs/console-authentication.md. Terminal branding and prompt templates live in terminal.json; see docs/terminal.md.


## Self-service passwords and capture arguments

At root, passwd (alias change_pass) verifies the current password and asks for
a new password twice, with no echo or history. It changes only the current
login: Linux/PAM for a mapped Console account, access/console/users.json for a
legacy Console identity, or access/ssh/users.json for SSH. It cannot target
another user or change roles. Key-only SSH users need an administrator to
provision a password before this old-password-verified flow is available.
The password service can be managed with termctl service status password-manager.

Use execute capture host 192.0.2.10 port 443 protocol tcp. Host or port remains
required; protocol is optional. Signed validators still constrain all values.
Typed ? still opens help and preserves input; its help row and Tab entry are hidden.
Intentional Ctrl+C stops are reported as stopped in production; typed port-probe
diagnoses are shown without a redundant generic error. show_result=true retains
the debugging result summaries.
