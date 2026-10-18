# SafeTerm configuration layout

Administrator input has one source: `/etc/safeterm/resources.d/*.json`.
Copy `resources.d/TEMPLATE.json` to a new filename, edit it, then run:

```sh
sudo termctl check
sudo termctl apply
```

The template is always ignored. Each active JSON definition can independently
contain `config`, `logs`, `service`, or a combination. See
[the complete reference](resources.d/README.md) for examples and field constraints.

```text
/etc/safeterm/
  README.md, README.txt
  resources.d/             Administrator definitions and inactive reference files
  ssh/                     SSH credentials and host key
  console/                 Internal Console credentials
  trust/                   Trusted catalog public key
/var/lib/safeterm/generated/ Root-owned compiled runtime data; never edit
/usr/share/safeterm/defaults/ Verified package built-ins; never edit
/var/lib/safeterm/audit/    Audit-service-owned JSONL data
/root/.config/safeterm/    Local signing private key, when selected
```

Generated registry data is `/var/lib/safeterm/generated/resources.json`.
There is no administrator `config-manager/resources.json`, operations directory,
or separate catalog activation workflow. `termctl apply` rebuilds the complete
runtime configuration from verified packaged built-ins and current JSON inputs.
It validates, signs, and activates catalog, registry and writer restrictions in
one recoverable transaction. Files being managed retain ownership and metadata.
No separate person approves an edit; permissions and the editing user's own
confirmation apply.

Use `sudo termctl apply --rollback` to restore the previous completed definition
activation, and `sudo termctl apply --recover` after an interrupted activation.
Neither changes source JSON files or application values. `termctl rollback`
is the separate package-installation rollback. In the restricted editor,
`rollback` restores the managed application's retained file backup.

This is a breaking architecture change (ADR-032). Installers reject old layouts
before deployment. They do not import, delete or silently reinterpret old files.
Back up an old installation and recreate its intended resources as new JSON
inputs on a clean deployment. Do not overlay an old `/etc/safeterm` backup.
On subsequent installations using this architecture, package updates preserve
custom JSON sources and identities and rebuild generated data. Copy only custom
resource definitions to another server; provision its files, units, access and
signer separately. Never copy credentials, private keys or compiled state.
