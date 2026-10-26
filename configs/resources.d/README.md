# Resource definitions

Requires a SafeTerm build containing ADR-032. Older releases do not load this
format. These files describe existing application resources, not Linux users or
new SafeTerm daemons. JSON is standard JSON: no comments or trailing commas.

## Start here

From an ordinary Linux administrator shell:

    sudo cp /etc/safeterm/resources.d/TEMPLATE.json /etc/safeterm/resources.d/myapp.json
    sudo nano /etc/safeterm/resources.d/myapp.json
    sudo termctl apply

Use `sudo termctl check` first if you want validation without activation. Apply
always performs the same checks. Give the definition a unique id, edit paths,
fields and roles, and remove the service/config/logs sections you do not need.
Existing roles are listed by `termctl role list`. A role mentioned here must
exist in packaged defaults or roles.d; definitions do not create users or grant
roles to users. roles.d, profiles.d, executables.d, operations.d and policies.d
are compiled by the same check/apply workflow (ADR-033).

TEMPLATE.json is ALWAYS ignored before opening/decoding its contents. Its exact,
case-sensitive filename is reserved. All other *.json files in this directory
are active inputs, regardless of their name. Rename an input to *.json.disabled
to disable it, then apply. Editor backups and non-.json files are ignored.
Package updates refresh TEMPLATE.json and these READMEs, preserve custom files,
and rebuild generated data from these definitions. Copy the template before
editing it. Deleting/renaming a source removes only its generated registrations
at the next successful apply; it does not delete application files or stop the
external application service.

## Independent sections

Each definition needs id, description and at least one of service, config, logs.
Identifiers use lowercase letters, digits and hyphens, start with a letter and
are at most 48 characters. Filenames do not have to match ids. Keep each file
root-owned, without group/world write permission; 0644 or 0600 is suitable.
Directories must be real, root-owned and not writable by groups/others. Active
symlinks, hardlinks, nonregular files, duplicate properties/ids, unknown fields,
trailing JSON, deep inputs and oversized inputs are rejected. Limits: 256 active
files, 1 MiB per file, 8 MiB total; generated metadata files are each <=1 MiB.

## Logical config contexts and field names

Register a physical file once. Its config.contexts can expose multiple logical
paths and field names without duplicating that file or writing Go code. The
inactive TEMPLATE.json uses this recommended structure. Do not mix config.fields
and config.contexts in one resource. Existing config.fields definitions still
use resource/section navigation; a resource with contexts exposes only its
logical paths, not an additional physical resource-ID entry point.

Example config-only definition:

```json
{
  "id": "network-options",
  "description": "Shared application settings",
  "config": {
    "path": "/etc/product/options.ini",
    "format": "ini",
    "permissions": {
      "read": ["config-viewer", "config-operator"],
      "edit": ["config-operator"],
      "rollback": ["config-operator"]
    },
    "contexts": [
      {
        "path": ["tunnel"],
        "fields": [
          { "name": "Value_1", "section": "Tunnel", "key": "TCPMSS", "type": "integer", "min": 1, "max": 65535 },
          { "name": "Value_2", "section": "Cat", "key": "Port_2", "type": "port" }
        ]
      },
      {
        "path": ["application", "cat"],
        "fields": [
          { "name": "enabled", "section": "Cat", "key": "Enabled", "type": "boolean" }
        ]
      }
    ]
  }
}
```

The bounds above are illustrative administrator policy. Field types/constraints
are the existing schema, declared once alongside the mapping. name is the exact
case-sensitive CLI field name; section/key are the exact real file coordinates.
For example set Value_1 1400 writes TCPMSS = 1400 in [Tunnel]. Values are not
translated; this maps names and coordinates only. Paths use 1..8 lowercase tokens
matching [a-z][a-z0-9-]{0,47}. Field names use letters, digits, dots, underscores
and hyphens, starting with a letter, up to 128 characters. A resource supports
up to 128 contexts and each context 1..512 fields. At most 4096 total routes are
accepted. Duplicate names in one context, repeated physical fields and ambiguous
or prefix-conflicting paths fail check/apply. Separate contexts may reuse a field
name when they refer to different physical keys.

After sudo termctl check and sudo termctl apply, reconnect and use:

    config tunnel
    show
    set Value_1 1400
    set Value_2 443
    diff
    end

    config application cat
    set enabled false
    end

? and Tab discover paths, names and typed choices according to resource roles.
Tab keeps its existing common-prefix/list behavior. A singleton context has direct
fields, so no edit/next is needed; use end to save and abort/cancel to discard.
There is no automatic entry creation or deletion. Legacy resource/section editing
continues to use edit/next and saves at end.

Each end validates the final physical document and saves once; it never restarts
or reloads a service. config.on_change is rejected; remove it and use execute
service ACTION ID separately with the appropriate role. A multi-context file has
one version/lock authority. External changes reject a stale save and retain the
draft; abort and reopen before reapplying. No force overwrite exists. Other views'
fields/comments are preserved. Cross-field checks still apply to the full file.

Resource read/edit/rollback roles cover every context of that file; contexts do
not create independent grants or field-level RBAC. Raw section/key names outside
the current view are not settable through that logical CLI. The manager/writer
still independently authorize the real resource and validate real coordinates.
Secret input remains hidden; show/diff redact it and set lines are not stored in
history. rollback restores a whole-file backup, including other contexts; its
confirmation explicitly says so. It is not a context-only undo.

## Existing resource/section definitions

Configuration only (no systemd service is created or restarted):

```json
{
  "id": "product-options",
  "description": "Product settings",
  "config": {
    "path": "/opt/product/options.ini",
    "format": "ini",
    "backup_keep": 5,
    "permissions": {
      "read": ["config-viewer", "config-operator"],
      "edit": ["config-operator"],
      "rollback": ["config-operator"]
    },
    "fields": [
      { "section": "Server", "key": "Port", "description": "Listening port", "type": "port" }
    ]
  }
}
```

Inside SSH or Console:

    config product-options
    edit Server
    show
    set Port 9090
    diff
    end

Field types: string, integer, port, boolean, enum, path, ipv4, ipv6, ip, cidr, ip_port.
Existing typed field schema
options remain supported: required, allow_empty, secret, read_only, enum, min/max,
min_length/max_length, pattern and path policy. Section and key names match the
actual file, case-sensitively. Use an empty section for root fields and edit - in
the terminal. Unknown/unregistered keys remain untouched and are not editable.
Set `secret: true` for sensitive fields; `set KEY` prompts with hidden input.
INI comments/order/layout, blank lines and a leading UTF-8 BOM are preserved. JSON uses the existing deterministic
renderer. The source definition is always JSON even when the managed file is INI.

Required and empty values are separate controls:

| required | allow_empty | Missing key | Existing blank value |
|---|---|---|---|
| false | false (default) | Allowed | Must satisfy the field type/constraints |
| true | false (default) | Rejected | Rejected |
| false | true | Allowed | Allowed |
| true | true | Rejected | Allowed |

For an optional IPv4 such as PATIP in a shared application INI:

```json
{ "section": "Sultan", "key": "PATIP", "type": "ipv4", "required": false, "allow_empty": true }
```

allow_empty permits exactly an empty string, not whitespace or invalid nonempty
values. It bypasses scalar type/length/pattern/path validation only for that empty
string; nonempty values remain fully validated and cross-field rules still apply.
JSON stores permitted empty values as "", including for port/integer/boolean;
it does not insert zero, false or null. Missing registered keys are not created
by the editor. Older definitions without allow_empty keep their existing behavior.
Draft validation, manager previews and the privileged writer enforce the same flag.

termctl check identifies the resource ID, managed file path, section, key, type,
required/allow_empty flags and numeric bounds for field errors. It distinguishes
missing keys, blank values and invalid typed values. Secret values are redacted.

Port example (inside config.fields):

```json
{ "section": "Server", "key": "Port", "type": "port", "required": true }
```

The port type is an integer with a built-in inclusive range of 1..65535; min/max
are unnecessary. Optional min/max may narrow this range but cannot widen it.
Zero, negatives, fractions, service names and values above 65535 are rejected.
INI uses the integer renderer; JSON stores a number. Draft, manager and writer
validation apply the same bounds. Numeric cross-field rules also support port.

Network field examples (inside config.fields):

```json
[
  { "section": "Server", "key": "ListenIPv4", "type": "ipv4", "required": true },
  { "section": "Server", "key": "ListenIPv6", "type": "ipv6", "required": true },
  { "section": "Server", "key": "PeerAddress", "type": "ip", "required": true },
  { "section": "Server", "key": "Endpoint", "type": "ip_port", "required": true },
  { "section": "Server", "key": "Network", "type": "cidr", "required": true }
]
```

Use lowercase type names. ipv4 accepts only IPv4; ipv6 accepts only IPv6; ip
accepts either family; cidr accepts an IPv4 or IPv6 address with prefix length.
ip_port accepts an IP literal plus a port in 1..65535, such as 127.0.0.1:2420
or [::1]:2420; brackets are mandatory for IPv6 endpoints. Endpoint hostnames,
zero ports, missing ports and IPv6 zones/mapped addresses are rejected.
Validation uses Go net/netip and performs no DNS lookup or network operation.
For address-only types, names, ports, brackets, surrounding whitespace, zone identifiers, IPv4-mapped
IPv6, invalid prefix lengths and the literal any are rejected. Empty values are invalid by default even when required is false. Set
allow_empty=true explicitly when the application permits a blank value. Unspecified addresses (0.0.0.0 and ::)
and /0 prefixes are valid; add pattern constraints if your policy forbids them.
Text length/pattern constraints can narrow these types; enum, numeric min/max
and path policies cannot be combined with them. Values retain their original
spelling; CIDR host bits are preserved rather than silently masking the address.
INI formatting/comments remain unchanged and JSON stores these values as strings.
The types are resource config field types, not execution-operation parameter types.

Omitting edit/rollback roles makes the resource read-only. Backup retention
(default 5) is 1..32. Optional profile selects application (default), database or
network; no new profile or daemon is created. Optional cross_field_rules use the
existing typed rules. These resources use the restricted privileged writer;
ownership, mode, ACLs and xattrs are preserved. Do not chown the application file.
The existing root-controlled parent/sensitive-path restrictions still apply.
Unsupported paths, symlinks, hardlinks or metadata fail validation; this command
does not silently grant permissions or weaken the namespace restrictions.

Logs only:

```json
{
  "id": "application-output",
  "description": "Application output",
  "logs": [
    { "id": "main", "path": "/var/log/product/output.log", "roles": ["log-viewer"] }
  ]
}
```

Inside SSH or Console:

    execute logs application-output main

Each log has its own role list. A generated operation contains one fixed path,
no caller-selected filesystem argument; it follows the last 50 lines and new
output, with existing stream limits. Ctrl+C stops it, Ctrl+] exits the session.
The file must already exist, be regular and single-link, have no symlink path
components, and be readable/traversable by the safeterm-log-reader service user
and its groups. Check tests that identity's read access without changing ACLs
or ownership. Hidden/private namespaces such as /home, /root, /tmp and /run are
rejected. If needed, provision narrow read/traverse ACLs yourself and maintain
those permissions in log rotation. Registering a log grants no new OS privileges.

Service only:

```json
{
  "id": "myapp",
  "description": "My application",
  "service": {
    "unit": "myapp.service",
    "actions": {
      "status": { "roles": ["service-viewer", "service-operator"] },
      "restart": { "roles": ["service-operator"] },
      "start": { "roles": ["service-operator"] },
      "stop": { "roles": ["service-operator"] },
      "enable": { "roles": ["service-operator"] },
      "disable": { "roles": ["service-operator"] }
    }
  }
}
```

    execute service status myapp
    execute service restart myapp
    execute service start myapp
    execute service stop myapp
    execute service enable myapp
    execute service disable myapp

Create the definition as /etc/safeterm/resources.d/myapp.json (service-only is
valid), then run sudo termctl check and sudo termctl apply from a normal Linux
admin shell. Reconnect SSH/Console after activation. Roles must exist and the
SafeTerm identity must hold each action's declared role. A service-viewer sees
status only in this example; service-operator sees all six actions. Use whoami
inside SafeTerm to inspect the current session roles. The identity name admin
does not imply unrestricted access. This works for external services such as
nginx.service as well as SafeTerm units; substitute an exact installed unit.

The unit must already be loaded. Supported actions: status/start/stop/restart/
reload/enable/disable. No action or role is implicit. No wildcards, shell commands
or dynamic unit parameters. Only exact signed unit/action pairs receive the
existing restricted systemctl grants. Only the dedicated service runner uses
those grants; users and Config Managers do not receive general sudo access.
`termctl service ...` still manages SafeTerm's own runtime daemons.

Combine service/config/log sections when convenient. Saving a configuration only
writes the file. Automatic service activation through config.on_change is no
longer supported; remove that property and use execute service separately.

## JSON syntax diagnostics

termctl check reports definition syntax/type errors with the filename, line,
character column and byte offset. Inspect that location in your JSON editor;
objects use braces and arrays use brackets. JSON does not accept trailing commas.
Diagnostics do not echo configuration values. Existing files are not rewritten
when validation fails.

## Activation, compatibility and recovery

One apply compiles registry fields, exact operations and role policies; checks
schemas, files, privileges, executable hashes, identities, writer sandbox and
sudoers; signs using the existing local key; then activates a journaled update.
No application values, ownership or ACLs are edited by definition activation.
Only previously active SafeTerm services are restarted, including SSH; use a
normal Linux admin shell and reconnect existing terminal sessions afterward.
Services deliberately inactive remain inactive. Repeating unchanged definitions
does not restart services. Failure restores the prior catalog, registry, writer
sandbox, generated inventory and active-service set.
A failure during recovery retains the journal and blocks installation/updates.

    sudo termctl apply --recover

This recovers an interrupted activation; it is not application-value rollback.
`sudo termctl apply --rollback` restores the previous completed activation,
including associated resource metadata. It does not edit your resources.d source
files: restore/edit the source before applying again. The terminal's rollback
restores the newest retained application-file backup.

Only verified packaged built-ins and current resources.d inputs are compiled.
Previous generated operations, registry entries and inventories are never source
inputs. Removing an active JSON file removes its registrations on the next apply.
An id or terminal-path conflict fails validation. Do not edit generated files.
The runtime data lives under /var/lib/safeterm/generated; credentials and trust
remain under /etc/safeterm. The audit service owns only /var/lib/safeterm/audit,
not the generated namespace.

Old layouts are explicitly refused by installation and activation (ADR-032).
There is no migration/import/fallback path. Back up and re-express desired
resources on a clean deployment; old files are never automatically deleted.

For offline signing, no private key is generated or imported automatically:

    sudo termctl apply --export /root/myapp-candidate

The parent must be root-controlled and the destination must not exist. Transfer
this unsigned catalog to the protected signer and sign with the installed trust's
key using safeterm-catalog-sign. Return the signed directory, then:

    sudo termctl apply --signed-catalog /root/myapp-signed

The signed catalog must exactly match the currently compiled definitions. Paths
and registry fields are independently validated locally; a signed catalog does
not bypass resource validation. `termctl check` needs no private signing key.

To reuse a setup on another server, copy your custom resources.d JSON files,
provision the referenced files/units, existing roles and narrow read permissions,
and apply there using that server's signing method. Do not copy users, private
keys, machine-specific hashes or generated manifests. Package updates preserve
custom source files and identities, then rebuild generated data. A package
upgrade needing a new signature requires the existing local signer; offline
operators must prepare matching signed inputs. It fails rather than silently
trusting or importing previously generated definitions.

Log registration
----------------
No sample log sources are enabled by the package. Register logs here using the
logs section in TEMPLATE.json; service and config sections are optional and can
be omitted for a log-only resource. Use execute logs RESOURCE_ID LOG_ID.
The log-viewer role and isolated log-reader profile are ready to use.
After changes run sudo termctl check, then sudo termctl apply and reconnect.
Older explicit operations.d/log.json overrides are preserved on upgrade; remove
them after migrating their paths here if you no longer want the old command.

Service aliases and ordered groups
----------------------------------
The resource id is the public service name. For example id web with
service.unit nginx.service exposes execute service restart web. Restricted
terminal responses contain the alias and step results, never raw systemctl or
journal output. Status reports active or inactive or failed; detailed diagnosis
is performed by the administrator using systemd and the protected Audit log.
Administrator-visible source files and audit records still contain real units;
aliases are presentation, not an operating-system confidentiality boundary.

A service can specify exactly one unit or an ordered units array (1..32 unique,
exact .service names). The array specifies STARTUP order, dependencies first.
Do not combine unit and units. Example /etc/safeterm/resources.d/akbar-all.json:

```json
{
  "id": "akbar-all",
  "description": "Akbar application stack",
  "service": {
    "units": ["akbar_1.service", "akbar_2.service", "akbar_3.service"],
    "timeout_seconds": 120,
    "actions": {
      "status": {"roles": ["service-viewer", "service-operator"]},
      "start": {"roles": ["service-operator"]},
      "stop": {"roles": ["service-operator"]},
      "restart": {"roles": ["service-operator"]},
      "enable": {"roles": ["service-operator"]},
      "disable": {"roles": ["service-operator"]}
    }
  }
}
```

Run sudo termctl check, sudo termctl apply and reconnect. Existing Tab and ?
completion offer only authorized action/alias pairs:

    execute service start akbar-all
    execute service restart akbar-all
    execute service status akbar-all

Start, enable, status and reload process members in array order. Stop and disable
process them in reverse. Group restart stops ALL members in reverse order before
starting ANY member in array order. A single unit retains normal systemd restart.
Enable/disable only change boot enablement; they do not implicitly start/stop.
Each synchronous systemctl step must finish before the next one starts.

No action or role is implicitly granted by a group. A restart-only group receives
exact stop/start sudoers pairs, not a wildcard or access to unrelated units.
The signed sequence order, alias and timeout are part of the catalog signature.

Total sequence timeout defaults to 120 seconds; timeout_seconds may be 1..240
(0 also selects the default). The Runner profile timeout independently bounds
each controller call and may be shorter. Stop at the first failed/timed-out step
or unavailable Audit service. Report the alias, step number and action; preserve
successful prior steps. No automatic retry or recovery starts other services.
An interrupted or failed sequence can leave a partially stopped/started group;
a timed-out systemctl client does not guarantee its systemd job was canceled.
An inactive/failed status is reported as a successful state query, not as a
successful service start. A missing unit or other controller failure fails the
query. Use systemd directly as administrator for detailed diagnosis.

Gateway reserves the complete exact-unit set across Runner profiles; Runner
also rejects local overlap. Conflicting requests fail rather than queue. A
caller disconnect does not release the reservation while a bounded sequence
continues. After uncertain transport failure, the reservation is retained through
the sequence deadline and cleanup allowance. Disjoint groups can run concurrently.
These guards cover SafeTerm operations, not manual systemctl calls, automatic
systemd activity, or implicit dependencies not listed in the group. Use canonical
unit names consistently: different systemd aliases of one unit are not resolved
into one lock key. Arrange the array consistently with systemd dependencies and
service readiness settings; SafeTerm does not infer an application dependency
DAG or application health checks. Gateway/Runner restarts are administrative
interruptions, not resumable transactions.

The protected Audit log records each step before execution and after its result,
with real unit/action, step number, request id and initiating identity. The
restricted user only sees the public alias. Renaming a resource changes its
public commands. Existing single-unit definitions need no schema change.

Audit step identity uses operation=service-step:PARENT_OPERATION:INDEX:ACTION:UNIT
with existing accepted/completed outcomes and the initiating request id. This
keeps the V1 audit envelope readable by older package binaries during rollback.

Service targets are registered exclusively by administrator `resources.d` definitions in a fresh installation. Packaged service roles and the isolated service-manager remain available, but have no default service operations or sudo grants. Manage SafeTerm itself from the Linux administrator shell with `sudo termctl service`. Existing explicit `operations.d`/`policies.d` overrides remain administrator-owned and must be reviewed if they refer to removed packaged service operation IDs.
