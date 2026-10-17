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

### 1. Check the target and install OS prerequisites

Use Ubuntu 24.04 with systemd running as PID 1 and an administrator account with
`sudo`. No Git, Go or Python build environment is required to install the Release.
Check the architecture:

```bash
uname -m
ps -p 1 -o comm=
```

`x86_64` needs **amd64**; `aarch64`/`arm64` needs **arm64**. The examples below use
amd64; replace both archive and extracted-directory names with arm64 when needed.
Download the archive **and its `.sha256` file from the same Release**. Use the
prebuilt assets, not GitHub's automatically generated "Source code" archive.

On an Internet-connected Ubuntu target, install prerequisites before SafeTerm:

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates coreutils findutils gawk grep gzip \
  libc-bin openssh-client passwd sed sudo systemd tar tcpdump util-linux \
  iputils-ping dnsutils traceroute procps
```

On an air-gapped host, provision these packages from installation media or an
internal APT mirror first. "Offline package" means SafeTerm's binaries are bundled;
it does not include Ubuntu packages. Plain containers without systemd as PID 1
are not covered by this installation procedure.

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

### 3. Create the initial SSH password hash (fresh installation only)

The installer requires an initial SSH credential. This example creates internal
SSH user `admin`, not a Linux account named `admin`. The bundled tool asks for the
password twice without echo and writes only a bcrypt hash:

```bash
sudo install -d -m 0700 /root/safeterm-secrets
sudo ./payload/bin/safeterm-password-hash \
  -output /root/safeterm-secrets/admin-bootstrap.hash
```

Use 8–72 printable ASCII characters, including uppercase, lowercase and a number.
Do not put the plaintext password in a command argument. The tool refuses to
overwrite an existing hash file; reuse the correct file or choose a new filename.
An SSH public key is optional; a key-only initial installation may instead supply
`--ssh-public-key /path/to/admin.pub`.

### 4. Preview and install

For a demo or single-host evaluation, use local signing. SafeTerm hashes the
approved executables on this host, signs the catalog locally, and keeps the key
under `/root/.config/safeterm` for later updates. Run the preview first:

```bash
sudo ./install.sh --local-signing \
  --ssh-user admin \
  --ssh-password-hash-file /root/safeterm-secrets/admin-bootstrap.hash \
  --dry-run
```

After `Dry run: PASS`, install:

```bash
sudo ./install.sh --local-signing \
  --ssh-user admin \
  --ssh-password-hash-file /root/safeterm-secrets/admin-bootstrap.hash
```

Expect `SafeTerm package installation: PASS`. The installer installs `termctl`,
configuration and service units, creates the isolated service accounts, and starts
the services. `--local-signing` alone is insufficient on a fresh host: the SSH
password-hash or public-key option is required. `--single-operator` is obsolete
and unnecessary.

For production with the signing key kept off the server, use the
[production offline signing procedure](#production-offline-signing)
instead of this local-signing step. The Release archive checksum verifies the transferred
package; catalog signing separately protects the deployed command definitions.

### 5. Verify services and test SSH

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
accepting a new host. Enter the SSH password from step 3. If a host firewall or
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

### 6. Create a Console user and test local login

Console and SSH have independent user lists and passwords. The SSH `admin` user
above does not automatically become a Console user. From the ordinary
administrator shell:

```bash
sudo termctl user add console --id alice --roles viewer,netops,config-viewer
sudo -iu safeterm
```

The first command asks for Alice's internal password twice. The second opens the
shared restricted Linux entry account, then SafeTerm prompts:

```text
Username: alice
Password: (the internal password just set; hidden)
safeterm::alice>
```

No Linux account named `alice` is needed. Fresh packages contain a disabled
`local-admin` placeholder and no default Console password. The Linux entry account
is initially password-locked; `sudo -iu safeterm` works for an authorized admin.
If direct physical Linux login is required, set its separate entry password with
`sudo passwd safeterm`; the internal SafeTerm login is still mandatory afterward.

To discover roles or change the internal password:

```bash
termctl role list
termctl role describe netops
sudo termctl user password console --id alice
```

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
For a fresh host, create the initial SSH hash with step 3 as well. From the package
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
  --ssh-password-hash-file /root/safeterm-secrets/admin-bootstrap.hash \
  --dry-run

sudo ./install.sh \
  --signed-catalog /media/transfer/safeterm-target-catalog \
  --ssh-user admin \
  --ssh-password-hash-file /root/safeterm-secrets/admin-bootstrap.hash
```

## Troubleshooting

Run administrative commands in the ordinary Linux administrator shell, outside
`safeterm::USER>`. Mutating `termctl` commands require `sudo`.

| Symptom | What to check |
|---|---|
| `termctl: command not found` | Confirm installation ended with `SafeTerm package installation: PASS`. Run `ls -l /usr/local/bin/termctl /usr/local/bin/safeterm-termctl` and try `sudo /usr/local/bin/termctl help` if PATH is the issue. If both are absent, use the package installer; do not assume a failed preflight installed anything. |
| `--ssh-password-hash-file or --ssh-public-key is required` | Supply the bootstrap credential from step 3 on a fresh host. Console credentials are created after installation. |
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
