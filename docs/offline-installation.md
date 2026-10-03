# Online and Air-Gapped Installation Package

SafeTerm can be built once on an Internet-connected Linux machine and installed
on an Ubuntu 24.04 target without Git, Go, module downloads, or Internet access.
The package contains statically built SafeTerm tools and services, catalog
sources, systemd units, example managed data, configuration schemas, a package
checksum manifest, and the host-side installer.

## Target prerequisites

The target must already provide systemd and these Ubuntu tools and executable
paths:

```text
sha256sum install useradd usermod groupadd getent systemctl ssh-keygen awk grep sed tar date find realpath visudo
/usr/bin/uname /usr/bin/tcpdump /usr/bin/tail /usr/bin/systemctl /usr/bin/sudo
```

Install missing OS packages from an Ubuntu installation medium or internal APT
mirror when the target has no Internet access. SafeTerm deliberately does not
embed distribution packages or silently alter APT sources.

The package is self-documenting for later customization. Commented catalog
templates are under `payload/catalog-source/`, and identity/configuration
registry templates are under `payload/templates/`. Files ending in `.jsonc`
are examples for administrators and are not loaded by SafeTerm.

## Build on the connected machine

Use Ubuntu 24.04 or WSL with Go 1.26 or newer. Check out the release you want to
ship and build a package for the target architecture:

```bash
git clone https://github.com/ASO8304/safeterm.git
cd safeterm
git fetch --tags
git checkout v0.20.1-m19

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
tar -C "$HOME" -czf "$HOME/safeterm-offline-amd64.tar.gz" \
  safeterm-offline-amd64
sha256sum "$HOME/safeterm-offline-amd64.tar.gz" \
  > "$HOME/safeterm-offline-amd64.tar.gz.sha256"
```

Transfer both files by SCP, removable media, or an approved internal transfer
system. Verify the outer archive checksum before extraction on the target.

## Quick single-host installation

This mode keeps a catalog signing key under `/root/.config/safeterm` so future
catalogs can be generated on the same isolated host. It is suitable for demos
and single-host evaluation. Production should use the two-stage offline signing
workflow below.

Create or provide an SSH client public key, then run a dry run and installation:

```bash
tar -xzf safeterm-offline-amd64.tar.gz
cd safeterm-offline-amd64

sudo ./install.sh \
  --local-signing \
  --ssh-user alice \
  --ssh-public-key /home/keyhan/safeterm-demo-client/alice_ed25519.pub \
  --single-operator \
  --dry-run

sudo ./install.sh \
  --local-signing \
  --ssh-user alice \
  --ssh-public-key /home/keyhan/safeterm-demo-client/alice_ed25519.pub \
  --single-operator
```

`--single-operator` makes each proposing role able to apply its own config
proposal. Omit it when separate approval is required. On upgrades, the installer
preserves the existing SSH host key, SSH and console identity maps, Config
Manager registry, and `/etc/safeterm-managed` contents. Therefore it does not
rotate the SSH host fingerprint or reset managed values.

Enable local console authentication explicitly:

```bash
sudo passwd safeterm
```

## Production offline signing

First prepare a catalog on the exact target host. This hashes the target's
installed `uname`, `tcpdump`, and `systemctl` binaries:

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
  --ssh-user alice \
  --ssh-public-key /home/keyhan/safeterm-demo-client/alice_ed25519.pub \
  --single-operator \
  --dry-run

sudo ./install.sh \
  --signed-catalog /media/transfer/safeterm-target-catalog \
  --ssh-user alice \
  --ssh-public-key /home/keyhan/safeterm-demo-client/alice_ed25519.pub \
  --single-operator
```

## Verification and use

```bash
sudo /usr/local/bin/safeterm-preflight --installed
sudo systemctl --no-pager --full status safeterm-ssh-broker.service
```

From the client:

```bash
ssh -tt -p 2222 \
  -i "$HOME/safeterm-demo-client/alice_ed25519" \
  -o IdentitiesOnly=yes \
  -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$HOME/safeterm-demo-client/known_hosts" \
  alice@SERVER_IP
```

The package checksum is verified before catalog preparation or host mutation.
The normal bundle preflight then verifies the signed catalog, identities,
resource registry, binaries, units, and managed seed layout before the base
installer changes the host.

## Transactional installation rollback

Every non-dry-run installation creates a root-only transaction before the first
persistent installation change. The transaction records the previous SafeTerm
files and directories, local signing-key directory, audit data, managed data,
service unit files, service enabled/running states, and the affected local
account databases. Its contents are protected by a SHA-256 manifest.

The installer prints the transaction ID and exact rollback command. To restore
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
