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
