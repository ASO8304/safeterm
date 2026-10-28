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

## Administrator layout

Settings are grouped under `/etc/safeterm/access/`, `commands/`, `resources.d/`,
`network/` and `trust/`. Guides live in `docs/`; the root README explains the tree.
The network policy is `network/policy.json`. Package updates migrate v0.27 flat
source directories and credentials automatically after taking the installation
snapshot. Contents, ownership and ACLs are preserved; conflicts stop the update
before host changes. Runtime services read only the grouped paths.

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

Operations may also define `required_any` (at least one named parameter must
have a nonempty effective value), `argument_groups` (each with `when` and
whole-token `args_template`), and a literal `argument_group_separator` array.
See [catalog authoring](catalog-authoring.md) for the shipped dynamic `trace`
example. Groups and constraints are signed configuration and remain subject
to normal typed validation, authorization and executable approval.

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
