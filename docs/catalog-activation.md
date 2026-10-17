# Apply catalog changes with termctl

Use this workflow after installation when changing approved log aliases, command
arguments, operations, catalog roles or policy grants. `termctl catalog apply`
validates, signs with an existing local key, saves a verified recovery baseline,
updates the exact service-manager sudoers grants, activates the catalog and checks
services. A Release package or Go compiler is not needed for catalog activation.
Run these commands from the ordinary administrator shell, not the restricted
`safeterm::USER>` prompt. Root is required.

Application values changed through `config edit` and users changed through
`termctl user` are separate configuration domains and do not need this command.
The published catalog and its detached signature still must match; the command
automates signing rather than disabling it.

## Already edited the installed catalog

If you added three log aliases to `/etc/safeterm/operations/operations.json`:

```sh
sudo termctl catalog apply --dry-run
sudo termctl catalog apply
```

The default source is `/etc/safeterm`. Invalid JSON, missing operations/roles,
incorrect executable hashes, unknown user/resource roles, an invalid sudoers plan,
or an untrusted signing key fail before any service is stopped. A dry run stages
and validates the signature and policy, but does not publish or restart services.
It does not test the subsequent systemd activation itself.

The installed local private key defaults to
`/root/.config/safeterm/catalog-ed25519.key`. For a deliberately configured custom
key location, add `--key-file /root/YOUR_KEY_DIR/catalog-ed25519.key`. The command
never generates a replacement key or changes the installed public trust anchor.
A missing local key requires an externally signed catalog, described below.

Normal installation/upgrade automatically saves the initial verified checkpoint.
If manually edited live JSON no longer matches its old signature, apply recovers
the rollback baseline from that checkpoint, not from the broken on-disk catalog.
If neither the installed catalog nor a saved checkpoint has a valid signature,
apply refuses activation: lost historical content cannot be reconstructed from
an old signature alone. Restore a valid catalog first. For a manually installed
binary or an installation using `--no-start`, initialize a checkpoint while the
installed catalog is still valid:

```sh
sudo termctl catalog checkpoint
```

The checkpoint command requires installed preflight to pass. It is normally an
installer implementation step, not something to run after each manual edit.
It resets catalog rollback history and must not replace a needed recovery point.

## Prefer editing a separate source copy

Keep the active catalog valid while working on changes:

```sh
sudo cp -a /etc/safeterm /root/safeterm-catalog-source
sudoedit /root/safeterm-catalog-source/operations/operations.json
sudo termctl catalog apply --source /root/safeterm-catalog-source --dry-run
sudo termctl catalog apply --source /root/safeterm-catalog-source
```

Use a fresh destination for the copy. It contains protected identity files as
well as catalog files; keep it under `/root`. Apply imports only catalog metadata
and the five catalog sections, never users, credentials, the trust public key,
managed application files, or the resource registry. JSON arrays are normalized
into one runtime file per section. Human `.jsonc` templates are left alone.
Keep a controlled source copy for subsequent edits and Release upgrades.

For the log example, add aliases to the `source` parameter's `value_map` in the
`log` operation, preserving the other aliases:

```json
"value_map": {
  "service_A": "/var/log/safeterm-readable/service_A.log",
  "app_one": "/var/log/app-one/application.log",
  "app_two": "/var/log/app-two/application.log",
  "app_three": "/var/log/app-three/application.log"
}
```

After applying, reconnect and use:

```text
execute log source=app_one
```

The operator still needs `log-viewer`, and the `safeterm-log-reader` account needs
read access and parent-directory traversal. Apply does not grant filesystem ACLs,
change log ownership or weaken systemd path restrictions. Log rotation must retain
that access. See [catalog authoring](catalog-authoring.md#add-an-allowlisted-log).

## Explicit executable hash refresh

Ordinary apply checks the existing approved digests; it does not silently trust
changed binaries. After reviewing an OS package update, use the existing explicit
hash-preparation command, then review and apply its result:

```sh
sudo termctl catalog regenerate-hashes \
  --source /etc/safeterm --output /root/safeterm-refreshed-catalog
sudo termctl catalog apply --source /root/safeterm-refreshed-catalog --dry-run
sudo termctl catalog apply --source /root/safeterm-refreshed-catalog
```

The output directory must not already exist. Adding a log alias does not require
refreshing the `tail` executable hash. Changing a profile's OS identity, socket,
capabilities, controller or resource limits requires a package/unit deployment;
catalog apply rejects those changes before activation.

## Offline signing

Prepare and review the candidate on the target, sign it with the existing approved
key on the protected signing host, then transfer the signed directory back. Apply
uses the target's installed trust anchor and ignores any different public key
included in the candidate:

```sh
sudo termctl catalog apply --signed-catalog /media/transfer/signed-catalog --dry-run
sudo termctl catalog apply --signed-catalog /media/transfer/signed-catalog
```

No local private key is needed in this mode. Signature verification still fails
closed. Trust rotation is a separate deliberate deployment action.

## Activation, failure recovery and rollback

A global lifecycle lock serializes catalog transactions with package installation,
update, rollback and uninstall. Apply records which catalog consumers are active,
leaves Audit running, stops active dependents first, replaces only catalog files
and the generated sudoers rule, runs installed preflight, and starts previously
active services in dependency order. Services that were inactive stay inactive.
There is no automatic service enablement. Active SSH/Console sessions disconnect,
and Console sessions must authenticate again after Identity restarts.

A failed publish, preflight or service start triggers automatic restoration of the
verified baseline and original running-service state. An unchanged valid catalog
with matching sudoers is a no-op and preserves rollback history. Successful changes
retain one previous catalog. To undo the latest catalog application:

```sh
sudo termctl catalog rollback
```

This differs from `sudo termctl rollback`, which restores an entire package
installation snapshot. Catalog rollback never restores account databases, users,
passwords or application data. It does require that current users/resources still
reference roles and operations available in the older catalog.

If the process crashes or automatic recovery itself fails, the pending journal
is retained. Further catalog applications and package lifecycle changes refuse
to proceed until recovery. After resolving the reported filesystem/systemd error,
run the same `sudo termctl catalog rollback` command; it restores the pending
transaction baseline without requiring a private signing key. There is no promise
of successful recovery while the underlying disk or service remains broken.

Checkpoints and the journal live under root-only
`/var/lib/safeterm-installer/catalog/`. They contain catalog data and generated
sudoers, not private signing keys or identity maps. They are included in package
installation rollback snapshots and retained by uninstall. A successful package
installation seeds its new baseline and resets catalog-only rollback history.
Package upgrades preserve the verified installed catalog by default. New package
defaults remain separate at `/usr/share/safeterm/defaults`. Explicit source/signed
input intentionally replaces catalog definitions. See
[configuration preservation and portability](configuration-portability.md).

Bash and Zsh completion include `catalog apply`, `catalog rollback` and apply
options. Existing shells may need to reload their generated completion script
following a binary update.


Portable export excludes host users, credentials, signatures, keys and the
Config Manager registry:

```bash
sudo termctl catalog export --output /root/site-catalog
# After copying the directory to a compatible, locally signed target:
sudo termctl catalog apply --source /root/site-catalog --refresh-hashes --dry-run
sudo termctl catalog apply --source /root/site-catalog --refresh-hashes
```

`--refresh-hashes` explicitly approves executables on that host. It cannot be used
with an offline-signed catalog. Default apply and update continue to enforce
existing approved hashes.
