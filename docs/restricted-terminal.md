# SafeTerm restricted terminal

The SSH and Console terminal use the same context-based command language.
These commands require a build containing ADR-030; older releases keep their
previous terminal interface. `termctl` remains the administrator's Linux utility.

## Discover commands

```text
safeterm::admin> ?
safeterm::admin> execute ?
safeterm::admin> execute service ?
safeterm::admin> execute service status ?
safeterm::admin> execute service status ssh ?
safeterm::admin> execute service status ssh
```

A separate `?` followed by Enter lists valid choices with short descriptions.
Tab completes commands, registered files, sections, fields and enum/boolean
values. Both features filter through the authenticated user's permissions.
No help request executes an operation or writes a file. Arrow keys navigate
history; Ctrl+C cancels input/stops a stream; Ctrl+] closes the entire session.

The root menu is `execute`, `config`, `whoami`, `clear`, `exit`.
`help` remains accepted for keyboard guidance but is not another primary menu.

## Logical configuration paths

A resources.d config.contexts definition can map config tunnel to named fields
such as Value_1 and Value_2, while storing them as [Tunnel].TCPMSS and [Cat].Port_2.
Use config ?, config application ? and Tab to discover authorized paths.

```text
safeterm::admin> config tunnel
safeterm::admin(tunnel)> show
safeterm::admin(tunnel)> set Value_1 1400
safeterm::admin(tunnel)> set Value_2 443
safeterm::admin(tunnel)> diff
safeterm::admin(tunnel)> end
Configuration saved.
```

Logical fields are directly settable without edit. A logical singleton has no
next; end saves and exits; abort or cancel discards the draft. show, diff, help,
Tab, typed validation and secret redaction use the logical names. Physical paths
and unrelated keys do not become user-selectable arguments. Multiple contexts
can share one registered file, with one version-checked atomic writer. Saving
never restarts services; use execute service separately. Whole-file rollback
can affect all views and asks for explicit confirmation.

See the [installed JSON guide](../configs/resources.d/README.md) for the complete
schema and examples. Existing config.fields resources retain section navigation
as described below. A source cannot mix fields with contexts, and a context-based
resource has no extra raw-ID editing path.

## Configure a file

The administrator first registers exact files and editable fields through
[resources.d](resource-definitions.md) and `termctl apply`. See
[external files](external-configuration-files.md) for that one-time setup.
Users never enter filesystem paths or arbitrary configuration keys.

```text
safeterm::admin> config ?
options                  Application settings
safeterm::admin> config options
safeterm::admin(options)> edit ?
Server                   Registered section
Logging                  Registered section
safeterm::admin(options)> edit Server
safeterm::admin(options/Server)> show
Port                             = 8080
safeterm::admin(options/Server)> set Port ?
Port                     integer min=1 max=65535
safeterm::admin(options/Server)> set Port 9090
safeterm::admin(options/Server)> next
safeterm::admin(options)> edit Logging
safeterm::admin(options/Logging)> set Level debug
safeterm::admin(options/Logging)> diff
Server.Port: 8080 -> 9090
Logging.Level: info -> debug
safeterm::admin(options/Logging)> end
Configuration saved.
```

- `config` or `config ?`: list accessible files.
- `config RESOURCE`: open one registered file and capture its current version.
- `edit SECTION`: select a section. Use `edit -` for root-level fields.
- `show`: show registered values, including staged replacements; hide secrets.
- `set KEY VALUE`: stage a scalar change; repeat it to replace the staged value.
  Use double quotes for an empty value or text requiring explicit quoting:
  `set Label "Production server"`. No shell evaluation takes place.
- `set SECRET_KEY`: enter a secret through a hidden prompt. Inline secret values
  are refused. `set` command lines are excluded from history.
- `diff`: show the local unsaved changes with secrets redacted. It is a draft
  comparison, not a promise that all server-side validation has passed.
- `next`: return to the file context, retaining changes across sections.
  Unlike FortiGate, this does **not** save an entry; SafeTerm saves the entire file
  as one transaction at `end`.
- `end`: validate all changes together, save once and leave the editor.
- `cancel`: discard all unsaved changes and leave the editor.
- `rollback`: with an empty draft, confirm restoration of the newest retained
  whole-file backup. This can revert other fields and comments to that snapshot.
- `exit` or Ctrl+]: close the session and discard unsaved changes.

`set` checks scalar types/ranges immediately. Full path, cross-field, application
and authorization checks run on the server at `end`. A valid final combination
can therefore be staged even if an intermediate combination violates a
cross-field rule. Up to 512 distinct fields and 256 KiB of draft values/coordinates
can be staged per session. One successful save creates one backup, if enabled,
and runs configured activation/health checks once. Existing rollback-on-failure
policy applies to the entire file update.

If another program changes the file after it was opened, `end` refuses the stale
version and retains the draft. Review `diff`, then `cancel`, reopen the resource
and enter the desired changes against the latest version. There is no force
option. Failed validation does not write some fields and leave others behind.

INI saves preserve unrelated comments, order, whitespace and line endings.
Existing privileged-writer ownership, ACL and metadata checks remain enforced.
JSON continues to use the deterministic JSON renderer.

## Execute commands

Common presentation names are:

| Terminal command | Signed catalog operation |
|---|---|
| `execute system_info host` | `system_info` |
| `execute system_info memory` | `memory_info` |
| `execute system_info disk` | `disk_usage` |
| `execute logs product main` | Generated resource operation with a fixed log path |
| `execute service status ssh` | `status_safeterm_ssh` |
| `execute service restart gateway` | `restart_safeterm_gateway` |
| `execute capture host=192.0.2.1` | `trace host=192.0.2.1` |
| `execute capture port=999` | `trace port=999` |
| `execute capture host=192.0.2.1 port=999 protocol=tcp` | `capture` with all three filters |

`execute ping host=127.0.0.1 count=5` retains its familiar name.
The packaged nslookup and traceroute operations are removed; capture replaces trace.
Service actions/targets appear only if their corresponding signed operations
exist and the role is authorized. Presentation aliases do not grant permissions,
change the signed catalog or run arbitrary Linux commands. Only the contextual names are accepted; raw legacy operation names, `list`,
`describe`, and one-shot configuration commands are removed.

## Administrator commands

```sh
sudo termctl check
sudo termctl apply
sudo termctl apply --rollback
sudo termctl apply --recover
```

These operate on resources.d definitions and generated permissions. The editor's
`rollback` restores application file values. Package `termctl rollback` restores
an installation snapshot. Ordinary users never handle signing keys.

## Log completion and service permissions

Logs are registered only through administrator definitions in
`/etc/safeterm/resources.d/*.json`. No default log sources are registered.
For a resource with id `product` and a log with id `main`, use
`execute logs product main`. Tab and `?` list only authorized registered
resources/logs; users cannot provide arbitrary filesystem paths. The inactive
`resources.d/TEMPLATE.json` shows the `logs` structure. See
[resource definitions](resource-definitions.md) for setup and file permissions.

Older installations may have an explicit `operations.d` override for the old
`log` operation. Remove that override and apply after migrating its paths to
resources.d; upgrades preserve administrator files. The built-in `log-viewer`
role and isolated log-reader profile remain available for resource-defined logs.

Service menus list only actions and units authorized for the session's roles.
`admin` is an identity name, not an implicit superuser. `whoami` shows the roles;
`service-viewer` grants the packaged status operations while `service-operator`
grants the packaged control operations. Adding a new service does not grant
access to all system units.

To expose an external unit, create a root-controlled JSON definition in
`/etc/safeterm/resources.d`, using the service-only example in its README.
Specify the exact unit (for example `nginx.service`) and status, restart, start,
stop, enable and disable actions with the intended role lists. Run
`sudo termctl check` followed by `sudo termctl apply` from a normal Linux shell,
then reconnect. The generated menu offers `execute service ACTION RESOURCE_ID`.
The service runner receives only those exact unit/action grants. `termctl service`
continues to administer SafeTerm's own runtime services.

The root-level show command is removed. Use execute COMMAND ? for command
help. The show command inside a configuration context still displays values.

Service resource IDs are public aliases for actual systemd units. A definition
may target one unit or an ordered group. Execute service restart GROUP stops all
members in reverse startup order and then starts them forward. The menu and Tab
behavior stay the same. Responses display the alias, step/action and state, not
raw systemctl output or physical unit names. An error stops the sequence without
automatic recovery; consult the administrator and protected Audit record for
physical-unit diagnostics. See the resource guide for group definitions.
