# Install a SafeTerm GitHub Release

Start here after downloading the prebuilt Release archive and its checksum. The
package contains statically built SafeTerm tools/services, catalog sources,
systemd units, example managed data, configuration schemas, a checksum manifest
and the installer. There is no Git checkout, Go compiler or module download on
the target. Building from source is an optional section at the end.

- [All Releases](https://github.com/ASO8304/safeterm/releases)
- [Download AMD64 package](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-amd64.tar.gz)
- [Download AMD64 checksum](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-amd64.tar.gz.sha256)
- [Download ARM64 package](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-arm64.tar.gz)
- [Download ARM64 checksum](https://github.com/ASO8304/safeterm/releases/latest/download/safeterm-offline-arm64.tar.gz.sha256)

### 1. Choose the package for your server

Use Ubuntu 24.04 with systemd running as PID 1 and an administrator account with
`sudo`. Download **amd64** for `x86_64`, or **arm64** for `aarch64`/`arm64`. No Git,
Go or Python is required on the target. Examples below use amd64.

The guided installer detects missing OS tools and installs them from the configured
APT repositories on Ubuntu/Debian. On an air-gapped server, provide prerequisites
through your internal APT mirror or installation media first. The offline SafeTerm
package bundles SafeTerm binaries, not Ubuntu packages. A container without systemd
as PID 1 is not a supported target.

### 2. Verify and extract the downloaded archive

Run these commands in the directory containing the downloaded archive and checksum.
The new staging directory avoids mixing a new Release with an old extracted copy:

```bash
sha256sum -c safeterm-offline-amd64.tar.gz.sha256
release_dir=$(mktemp -d "$HOME/safeterm-release.XXXXXX")
tar -xzf safeterm-offline-amd64.tar.gz -C "$release_dir"
cd "$release_dir/safeterm-offline-amd64"
ls install.sh PACKAGE-SHA256SUMS TARGET_ARCH payload/bin/safeterm-termctl
```

Stop if checksum verification fails. Keep this shell in the extracted package
root for the following steps. If SafeTerm is **already installed**, skip fresh
password enrollment and use **Updating an existing installation** below.

### 3. Install with one command

This guided path requires a package containing ADR-028. Release v0.20.14 and older
retain their original explicit-flag installation procedure.

After extracting the archive, open a terminal in the extracted package directory
and run:

```bash
sudo ./install.sh
```

On a fresh server the installer asks:

```text
SafeTerm username [admin]:
Password:
Confirm password:
```

Press Enter to use `admin`, or choose another SafeTerm identity. The password is
hidden and must contain 8-72 printable ASCII characters, uppercase, lowercase and
a number. The installer hashes it directly; no manual hash file is needed and no
plaintext password is passed on a command line or saved in configuration.

The guided path uses local catalog signing. It installs missing prerequisites,
validates the prepared deployment, installs the services and `termctl`, and starts
SafeTerm. A successful installation ends with `SafeTerm package installation: PASS`
and login instructions. Package integrity, catalog verification and deployment
preflight run automatically; a separate preview command is optional.

The chosen identity is enrolled in both the SSH and Console user lists using the
password you entered. These are independent credentials afterward; changing one
does not change the other. No Linux account is created for this identity.
On an existing installation the same command updates the runtime while preserving
its verified catalog, users, passwords and managed values; it does not ask for a
new bootstrap password.

For a preview use `sudo ./install.sh --dry-run`. It does not install OS packages,
modify live configuration or start services; missing prerequisites must be
provisioned before preview can succeed. Fresh preview enrollment remains temporary.

Organizations requiring a signing key off the target should use the explicit
[production offline signing procedure](#production-offline-signing).

### 4. Verify services and test SSH

Run from the ordinary administrator shell:

```bash
command -v termctl
sudo /usr/local/bin/safeterm-preflight --installed
sudo termctl service status all
```

`termctl` is installed at `/usr/local/bin/termctl`. From another machine, replace
`SERVER_IP` with the target address:

```bash
ssh -tt -p 2222 admin@SERVER_IP
```

Check the displayed host-key fingerprint through a trusted channel before
accepting a new host. Enter the SSH password chosen during installation. If a host firewall or
network ACL is active, allow TCP port 2222 only from the intended client networks.

Inside the restricted SafeTerm terminal, try:

```text
whoami
execute ?
execute system_info
execute memory_info
execute ping host=127.0.0.1 count=5
config list
exit
```

Tab completes authorized inputs; a standalone `?` followed by Enter shows help.
Ctrl+C stops an active stream; Ctrl+] exits the whole SafeTerm session.

### 5. Test Console login and add users

After guided installation, open the shared restricted entry account:

```bash
sudo -iu safeterm
```

Enter the SafeTerm username and password chosen during installation. With the
default name, the resulting prompt is `safeterm::admin>`.

Console and SSH have independent user lists and passwords after initial enrollment.
Additional Console users can be enrolled separately:

```bash
sudo termctl user add console --id alice --roles viewer,netops,config-viewer
```

This asks for Alice's internal password; it does not create a Linux account named
`alice`. The Linux entry account `safeterm` is initially password-locked;
`sudo -iu safeterm` works for an authorized administrator. Direct physical Linux
login additionally requires a separate entry-account password set with
`sudo passwd safeterm`; the internal SafeTerm login remains mandatory.

Explicit unattended/key-only installs keep the disabled `local-admin` Console
placeholder unless an existing Console user database is present. Enroll Console
identities with `termctl user add console` in that mode.

To discover roles or change the internal password:

```bash
termctl role list
termctl role describe netops
sudo termctl user password console --id alice
```

### Unattended installation and existing automation

The guided path requires a terminal. For Ansible or fleet provisioning, retain
explicit signing and bootstrap options and use `--non-interactive`. Create a
protected bcrypt hash through your provisioning/secrets process; the bundled
helper can also create it interactively:

```bash
sudo ./payload/bin/safeterm-password-hash -output /root/admin-bootstrap.hash
sudo ./install.sh --non-interactive --local-signing --ssh-user admin \
  --ssh-password-hash-file /root/admin-bootstrap.hash
```

Add `--install-dependencies` to explicitly allow APT when provisioning without a
terminal; otherwise missing prerequisites produce an error instead of an implicit
package installation. Key-only bootstrap remains supported with
`--ssh-public-key FILE`. Public keys are optional in the password-based guided path.
Never put plaintext passwords into command arguments. Existing installation flags,
`--signed-catalog`, `--catalog-source`, `--no-start` and `--dry-run` remain supported.

### Updating an existing installation

Verify and extract the **new** archive using step 2, then run these commands from
its package root. For an installation using the default local signing key:

```bash
sudo termctl update "$PWD" --dry-run
sudo termctl update "$PWD"
sudo /usr/local/bin/safeterm-preflight --installed
sudo termctl service status all
```

`$PWD` must contain `install.sh`, `PACKAGE-SHA256SUMS`, `TARGET_ARCH` and `payload/`;
do not pass the archive, `payload/`, or the parent download directory. The update
preserves the installed signed catalog (including customized operations and log
aliases), identities, the SSH host key, the Config Manager registry and existing
managed files with their ownership and permissions. Release catalog defaults are
kept separately at `/usr/share/safeterm/defaults`. It restarts services and interrupts active sessions; use an
ordinary administrator session for the update.

An unchanged offline-signed catalog is also retained without a signing key. Only
when intentionally replacing the catalog with another reviewed signed version, use:

```bash
sudo termctl update "$PWD" --signed-catalog /path/to/signed-catalog --dry-run
sudo termctl update "$PWD" --signed-catalog /path/to/signed-catalog
```

To reuse customized operations and log aliases on other servers, see
[configuration preservation and portability](configuration-portability.md).
Export with `sudo termctl catalog export --output /root/my-catalog`; on a compatible
locally signed target, use `sudo termctl catalog apply --source /root/my-catalog
--refresh-hashes --dry-run`, then repeat without `--dry-run`. This transfers
catalog definitions without cloning users or keys. This behavior requires a new
package containing ADR-027; older release installers are unchanged.

For an older Console OS-user mapping (for example, `user_id: safeterm`), set its
internal password once after upgrading; do not add the same ID again:

```bash
sudo termctl user password console --id safeterm
sudo -iu safeterm
```

Log in at the SafeTerm prompt using username `safeterm` and that internal password.
The old Linux account password does not replace internal authentication.

### Rollback and uninstall

From the ordinary administrator shell:

```bash
sudo termctl rollback
sudo termctl uninstall
```

These are separate actions: rollback restores the latest retained pre-install
snapshot; uninstall removes runtime binaries/units while retaining configuration,
accounts and audit data. Rollback also discards changes since the snapshot and
restores captured account databases, so use it before unrelated host account
changes. No transaction ID needs to be saved. See the full installation and
termctl guides for recovery after a partially failed installation.

## Production offline signing

Use the OS prerequisites and verified, extracted Release from steps 1–2 above.
For a fresh offline-signed host, create the initial SSH hash using the unattended-installation helper shown above. From the package
root on the exact target host, prepare a new catalog (the output directory must
not already exist). This hashes every registered executable, including the
network diagnostics, service control and log-reader binaries:

```bash
./install.sh --prepare-catalog /tmp/safeterm-target-catalog
tar -C /tmp -czf /tmp/safeterm-target-catalog.tar.gz safeterm-target-catalog
```

Move that archive to the protected signing machine together with the SafeTerm
package. Extract it and generate the deployment key only once:

```bash
mkdir -p /secure/safeterm
chmod 700 /secure/safeterm

./payload/bin/safeterm-catalog-sign \
  -generate-key \
  -private-key /secure/safeterm/catalog.key \
  -public-key /tmp/safeterm-target-catalog/trust/catalog-ed25519.pub

./payload/bin/safeterm-catalog-sign \
  -config /tmp/safeterm-target-catalog \
  -private-key /secure/safeterm/catalog.key \
  -signature /tmp/safeterm-target-catalog/catalog.sig
```

Keep `/secure/safeterm/catalog.key` off the runtime host. Return only the signed
catalog directory and install it on the target:

```bash
sudo ./install.sh \
  --signed-catalog /media/transfer/safeterm-target-catalog \
  --ssh-user admin \
  --ssh-password-hash-file /root/admin-bootstrap.hash \
  --dry-run

sudo ./install.sh \
  --signed-catalog /media/transfer/safeterm-target-catalog \
  --ssh-user admin \
  --ssh-password-hash-file /root/admin-bootstrap.hash
```

## Troubleshooting

Run administrative commands in the ordinary Linux administrator shell, outside
`safeterm::USER>`. Mutating `termctl` commands require `sudo`.

| Symptom | What to check |
|---|---|
| `termctl: command not found` | Confirm installation ended with `SafeTerm package installation: PASS`. Run `ls -l /usr/local/bin/termctl /usr/local/bin/safeterm-termctl` and try `sudo /usr/local/bin/termctl help` if PATH is the issue. If both are absent, use the package installer; do not assume a failed preflight installed anything. |
| `--ssh-password-hash-file or --ssh-public-key is required` | Use the guided `sudo ./install.sh` in a terminal, or supply a bootstrap hash/public key for unattended installation. |
| `invalid package directory` | Pass the extracted package root, for example `sudo termctl update "$PWD"` after `cd` into the new package. Check `ls PACKAGE-SHA256SUMS install.sh`. |
| `no local signing key; provide --signed-catalog DIR` | An older installed termctl requires a key even for unchanged updates. Use the new package's `sudo ./install.sh --dry-run` then `sudo ./install.sh` to preserve the installed verified catalog without a key. Explicit catalog replacement still requires the appropriate signing mode; never replace deployment trust casually. For a deliberately configured custom local key directory, use `sudo ./install.sh --local-signing --local-key-dir /root/YOUR_KEY_DIR` from the new package, with `--dry-run` first. |
| `password-hash: cannot create output` | Its parent directory must exist; the tool also refuses to overwrite an existing file. Choose another root-protected filename or reuse the intended hash. |
| `Permission denied (publickey)` | Check that the SSH identity has a password hash if you expect password login. Use the SSH username, not an internal Console-only user. |
| SSH `Connection refused` or a failed service | Check `sudo termctl service status ssh` and `sudo journalctl -u safeterm-ssh-broker -n 50 --no-pager`. A timeout may instead indicate a firewall/routing issue. |
| Console login fails after upgrading a legacy mapping | Set its internal password using `sudo termctl user password console --id EXISTING_ID`; Linux `passwd` changes only the entry account. |

The installer verifies its internal manifest before catalog preparation or host
mutation, then validates the catalog, identities, resource registry, binaries,
units and managed seed layout. Local templates are in `payload/catalog-source/`
and `payload/templates/`; `.jsonc` examples are not runtime configuration.

## Transactional installation rollback

For installed hosts, use `sudo termctl update /path/to/extracted/package`
(or add `--signed-catalog DIR` for offline signing), `sudo termctl rollback`,
and `sudo termctl uninstall`. The latter removes the runtime while preserving
state and accounts for reinstall. See [termctl](termctl.md) for details.
Successful installations retain only the latest recovery snapshot.

Every non-dry-run installation creates a root-only transaction before the first
persistent installation change. The transaction records the previous SafeTerm
files and directories, local signing-key directory, audit data, managed data,
service unit files, service enabled/running states, and the affected local
account databases. Its contents are protected by a SHA-256 manifest.

The installer prints the transaction ID and exact rollback command. No ID
needs to be saved: `termctl rollback` selects the latest snapshot. To restore
the newest pre-install state, run:

```bash
sudo /usr/local/sbin/safeterm-install-rollback latest
```

To select a specific retained transaction:

```bash
sudo /usr/local/sbin/safeterm-install-rollback TRANSACTION_ID
```

Rollback verifies the snapshot before changing the host, stops and disables
the installed SafeTerm services, removes the current SafeTerm installation
footprint, restores the snapshot, reloads systemd, and returns each service to
its previous enabled and active state. A fresh-install rollback therefore also
removes the SafeTerm accounts, home directory, local signing key, configuration,
managed data, audit data, binaries, units, and enablement links created by that
installation. The consumed transaction is deleted after a successful restore.

If installation stops after the transaction is created but before the rollback
command is installed, run the copy shipped in the extracted package:

```bash
sudo ./payload/rollback.sh latest
```

Run rollback in a maintenance window before making unrelated user, group, or
SafeTerm data changes. Returning to the exact pre-install state intentionally
discards changes made inside the managed installation paths after the snapshot.
System journal entries produced by installation remain part of the host's audit
history and are not erased.

Public-key authentication is optional. See [SSH authentication](ssh-authentication.md) for enrollment and upgrade behavior.

## Optional: build your own package from source

Use Ubuntu 24.04 or WSL with Go 1.26 or newer. Check out the release you want to
ship and build a package for the target architecture:

```bash
git clone https://github.com/ASO8304/safeterm.git
cd safeterm
git fetch --tags
git checkout v0.20.14 # Example: choose the release tag you intend to build

deploy/package/create-package.sh \
  --arch amd64 \
  --output "$HOME/safeterm-offline-amd64"
```

The equivalent Make target is:

```bash
make offline-package \
  OUTPUT="$HOME/safeterm-offline-amd64" \
  ARCH=amd64
```

For an ARM64 target, use `--arch arm64`. The output directory is complete and
does not depend on the source checkout after it is created. Archive it for
transfer:

```bash
cd "$HOME"
tar -czf safeterm-offline-amd64.tar.gz safeterm-offline-amd64
sha256sum safeterm-offline-amd64.tar.gz > safeterm-offline-amd64.tar.gz.sha256
```

Transfer both files by SCP, removable media, or an approved internal transfer
system. Verify the outer archive checksum before extraction on the target.
