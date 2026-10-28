# Define configuration, logs and services

The installed [resources.d guide](../configs/resources.d/README.md) is the full
reference. It is shipped as both README.md and README.txt under
/etc/safeterm/resources.d, together with an always-inactive TEMPLATE.json.

This feature requires a build containing ADR-032; it is not in v0.20.15.

```bash
sudo cp /etc/safeterm/resources.d/TEMPLATE.json /etc/safeterm/resources.d/myapp.json
sudo nano /etc/safeterm/resources.d/myapp.json
sudo termctl apply
```

Remove template sections you do not need. Each JSON file can independently
register a configuration file, one or more logs, a systemd service, or any
combination. Read [the guide](../configs/resources.d/README.md) for complete
config-only, log-only and service-only examples and their role/file requirements.

Use termctl check for optional validation without activation. Apply repeats
validation, compiles exact capabilities and signs/activates them as a recoverable
transaction. No separate config apply/catalog apply is needed for resources.d.
Legacy definition paths and mutation commands are removed. Only verified package
built-ins and current JSON .d inputs are compiled. Roles, operations, executables,
policies and Runner profiles use the additional [site definition directories](site-configuration.md)
introduced by ADR-033.
For application value editing see [the terminal guide](restricted-terminal.md).

Changed definition activation may disconnect SSH/Console sessions; invoke it from
a normal Linux administrator shell. No application values, owners or ACLs change.
Read the reference's recovery and offline-signing sections before using those
advanced modes. Custom source files survive package update; moving definitions to
another server requires that server's files, units, roles, read access and signer.

## Logical contexts

config.contexts maps logical CLI paths and field name aliases to physical
section/key coordinates. The [resource guide](../configs/resources.d/README.md)
contains the full JSON schema example, limits and permission behavior.
End saves files only; config.on_change is rejected. Manage services explicitly
through execute service. Existing config.fields definitions remain supported.

## Logs have one registration path

Packaged defaults do not register any log sources. Define application logs in
resources.d; no commands/operations.d override is needed. The log-viewer role, tail
executable binding and isolated log-reader profile remain available.

For example, create /etc/safeterm/resources.d/product.json:

```json
{
  "id": "product",
  "description": "Product logs",
  "logs": [
    {
      "id": "main",
      "description": "Main application log",
      "path": "/var/log/product/application.log",
      "roles": ["log-viewer"]
    }
  ]
}
```

After granting the log reader only the necessary read/traverse access, run
sudo termctl check and sudo termctl apply, reconnect and use
execute logs product main. Tab and contextual help list only authorized logs.
The filename above is an example: the log file must already exist on the host.

Upgrades preserve administrator inputs. If an older installation has an explicit
commands/operations.d override defining the old log operation (often log.json), migrate
its paths into resources.d and remove that override before applying. Merely
renaming that file to .json.disabled is also sufficient to exclude it. The new
package does not restore the old built-in operation. Other registered logs and
the actual log files are not removed.

## Service aliases and ordered groups

The resource id is the public service alias; service.unit is the physical unit.
For a group use service.units in startup order instead of service.unit. Start
runs forward, stop runs backward, and restart completes all reverse stops before
all forward starts. Only explicitly granted actions appear in Tab/contextual help.
Output uses the alias; full systemd status and journal text are not forwarded.

See [the installed resource guide](../configs/resources.d/README.md#service-aliases-and-ordered-groups)
for the complete JSON example, timeouts, partial-failure behavior, exact grants,
Audit records and concurrency limits. Single-unit definitions remain supported.

Service targets are registered exclusively by administrator `resources.d` definitions in a fresh installation. Packaged service roles and the isolated service-manager remain available, but have no default service operations or sudo grants. Manage SafeTerm itself from the Linux administrator shell with `sudo termctl service`. Existing explicit `commands/operations.d`/`access/policies.d` overrides remain administrator-owned and must be reviewed if they refer to removed packaged service operation IDs.
