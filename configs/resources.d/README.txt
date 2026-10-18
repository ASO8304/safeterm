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
      { "section": "Server", "key": "Port", "description": "Listening port", "type": "integer", "min": 1, "max": 65535 }
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

Field types: string, integer, boolean, enum, path. Existing typed field schema
options remain supported: required, secret, read_only, enum, min/max,
min_length/max_length, pattern and path policy. Section and key names match the
actual file, case-sensitively. Use an empty section for root fields and edit - in
the terminal. Unknown/unregistered keys remain untouched and are not editable.
Set `secret: true` for sensitive fields; `set KEY` prompts with hidden input.
INI comments/order/layout, blank lines and a leading UTF-8 BOM are preserved. JSON uses the existing deterministic
renderer. The source definition is always JSON even when the managed file is INI.

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
      "restart": { "roles": ["service-operator"] }
    }
  }
}
```

    execute service status myapp
    execute service restart myapp

The unit must already be loaded. Supported actions: status/start/stop/restart/
reload/enable/disable. No action or role is implicit. No wildcards, shell commands
or dynamic unit parameters. Only exact signed unit/action pairs receive the
existing restricted systemctl grants. Only the dedicated service runner uses
those grants; users and Config Managers do not receive general sudo access.
`termctl service ...` still manages SafeTerm's own runtime daemons.

Combine sections in one definition when convenient. Automatic application
reload is OFF by default, even if service and config occur together. To opt in,
set config.on_change to restart or reload and explicitly define that service
action and its roles. Saving then requires both edit and action permissions;
backup/recovery uses the existing whole-file rollback-on-activation-failure policy.
A configuration-only definition cannot specify on_change.

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
