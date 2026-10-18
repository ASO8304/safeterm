> Historical workflow, superseded by ADR-032. Its write commands and source
> paths are removed. Use [resource definitions](resource-definitions.md) and
> [termctl](termctl.md) for the supported interface.

# Preserve and reuse a customized deployment

Package updates preserve the installed, signed catalog, including customized
operations, log aliases, executable definitions, roles and policies. They also
preserve SSH and Console identities, the SSH host key, Config Manager resource
definitions and existing managed application files. New package catalog defaults
are reference material at `/usr/share/safeterm/defaults`; they are never merged
into the active permissions automatically.

This behavior is implemented after v0.20.14. Download a Release containing
ADR-027; an older downloaded installer retains its older update behavior. With an
older termctl that demands a local signing key, run the **new package's**
`sudo ./install.sh --dry-run`, then `sudo ./install.sh` to preserve an existing
offline-signed installation.

## Update an existing server

Finish intended edits before updating:

```bash
sudo termctl catalog apply --dry-run
sudo termctl catalog apply
cd /path/to/extracted/new-package
sudo termctl update "$PWD" --dry-run
sudo termctl update "$PWD"
```

Skip catalog apply when nothing has changed. An ordinary update reuses the
existing signature and trust, so it needs neither a local private key nor a newly
signed catalog. Unapplied edits, changed executable hashes, invalid role references
or incompatible profile boundaries abort validation before installation changes
and service stops. The administrator must review and resolve these conditions;
the installer does not reset customizations to make an update succeed.

The catalog is normalized into one JSON file per section during staging. Its
definitions and permissions are preserved; custom fragment filenames are not.
Unrelated files under `/etc/safeterm` keep their contents and permissions.
Existing managed files keep their contents, owner, group, ACLs and inode; package
seeds are used only for missing files.

## Reuse operations and log aliases on another installed server

On the configured source server:

```bash
sudo termctl catalog export --output /root/my-safeterm-catalog
```

The output directory must not exist. Copy this directory to the other server
using your normal administration tooling. It contains only `catalog.json` and
operations, executables, profiles, roles and policies. It excludes passwords,
users, SSH host keys, signing keys, signatures and Config Manager resource data.

On a compatible target installed with local signing:

```bash
sudo termctl catalog apply --source /root/my-safeterm-catalog --refresh-hashes --dry-run
sudo termctl catalog apply --source /root/my-safeterm-catalog --refresh-hashes
```

This replaces the target's catalog definitions with the exported set, validates
the target's existing user/resource references, hashes its installed executables,
signs with its own existing key, regenerates exact service-control permissions and
coordinates service restarts. It keeps the target's identities and trust anchor.
`--refresh-hashes` explicitly approves the target's executable content; use it
only after checking the target's installed OS packages. Ordinary apply/update
never refreshes approved hashes implicitly. A dry run writes no live config or
service state.

For a fresh host, the new package accepts the same source:

```bash
sudo ./install.sh --local-signing --catalog-source /root/my-safeterm-catalog \
  --ssh-user admin --ssh-password-hash-file /root/admin.hash --dry-run
sudo ./install.sh --local-signing --catalog-source /root/my-safeterm-catalog \
  --ssh-user admin --ssh-password-hash-file /root/admin.hash
```

Generate the hash using the package's password-hash helper as described in the
[installation guide](offline-installation.md). Fresh bootstrap roles must exist
in the imported catalog. Existing-host update can explicitly replace its catalog
with `sudo termctl update PACKAGE_DIR --catalog-source DIR`; this mode requires
the target's default local signing key and refreshes target executable hashes.

For offline signing, prepare on the target using
`./install.sh --prepare-catalog /root/prepared --catalog-source DIR`, sign with
the key corresponding to the target's approved trust anchor on the signing host,
then use `sudo termctl catalog apply --signed-catalog /root/signed` or the new
package's `--signed-catalog` installation mode. Do not copy a different host's
trust key or private key into `/etc/safeterm`.

## Host-specific files and recovery

Log aliases name absolute paths. The logs must exist on the target and its log
reader must have access; catalog transfer does not transfer log contents, ACLs,
logrotate configuration or grant new filesystem permissions.

If Config Manager resource definitions are also customized, provision the reviewed
`/etc/safeterm/config-manager/resources.json` separately as a root-owned file and
provision its application files at the declared paths with the correct ownership.
That registry is host-specific and is preserved by updates, but is not part of a
signed catalog export. Never clone all of `/etc/safeterm` across machines because
it also holds authentication and host identity material.

`sudo termctl catalog rollback` restores the previous catalog activation.
`sudo termctl rollback` restores the preceding package installation snapshot,
including the configuration at that update. Neither operation means “load factory
defaults”. Review changes made since the snapshot before rolling back a package.
