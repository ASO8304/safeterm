# SafeTerm Ansible Installer

This installer turns the verified offline SafeTerm release package into a
repeatable, one-command Ubuntu 24.04 deployment. It does not build binaries on
the target and does not bypass the package's signature, checksum, preflight, or
transactional rollback controls.

## Controller preparation

Install Ansible Core 2.15 or newer on a machine that can reach the target over SSH. Build or obtain
the official release package, then archive its directory without changing the
top-level name:

```sh
tar -C "$HOME" -czf safeterm-offline-amd64.tar.gz safeterm-offline-amd64
```

Copy the examples and edit the copies:

```sh
cd deploy/ansible
cp inventory.example.yml inventory.yml
cp group_vars/all.example.yml group_vars/all.yml
mkdir -p files
cp /path/to/safeterm-offline-amd64.tar.gz files/
```

At minimum, set the target address and SSH account in `inventory.yml`, and set
`safeterm_package_archive` plus the bootstrap password-hash path (or an optional public-key path) in
`group_vars/all.yml`. The SSH account must be able to use `sudo`/Ansible become.

Run the complete installation:

```sh
./install.sh
```

The wrapper runs `ansible-playbook` with the local configuration, inventory,
and variables file. Override their paths without editing the repository:

```sh
./install.sh --inventory /secure/inventory.yml --vars /secure/production.yml
```

## Dependency modes

- `online` installs missing packages from the target's configured Ubuntu APT
  sources.
- `repository` first adds `safeterm_apt_repository`, which may point to a local
  mirror reachable only on the deployment network, then installs dependencies.
- `preinstalled` performs no APT network operation. It verifies every required
  executable and stops before installation if the prepared image is incomplete.

The application archive is always copied from the Ansible controller, so the
target itself never needs GitHub or Internet access.

## Catalog signing

`safeterm_signing_mode: local` is suitable for demonstrations and a single
managed server. Its catalog private key remains under
`/root/.config/safeterm` on the target.

For production, prepare and sign the catalog through the package's offline
signing workflow, archive the returned signed directory, and configure:

```yaml
safeterm_signing_mode: offline
safeterm_signed_catalog_archive: /secure/releases/signed-catalog.tar.gz
safeterm_signed_catalog_root_name: signed-catalog
```

The playbook verifies the archive transfer and the package installer verifies
the catalog signature before host mutation.

## Configuration ownership

Empty `safeterm_ssh_users`, `safeterm_console_users`, and
`safeterm_config_registry_file` values preserve the installed files. Once one
of these values is defined, Ansible owns the complete corresponding file. JSON
is checked before replacement, affected services are restarted in dependency
order, and installed preflight verifies the resulting deployment.

Use Ansible Vault for inventory secrets. Do not put private SSH keys, signing
private keys, passwords, or tokens in this directory.

## Idempotence, verification, and rollback

The role records a fingerprint containing the package SHA-256, signing input,
and installation mode. Re-running the same release skips the transactional
package installation while still enforcing optional managed configuration and
verification. Set `safeterm_force_install: true` only when a new transaction is
deliberately required.

Run verification by itself:

```sh
ANSIBLE_CONFIG=ansible.cfg ansible-playbook -i inventory.yml verify.yml
```

Every package mutation creates a complete rollback transaction. Restore the
latest transaction with:

```sh
./rollback.sh
```

Or select the transaction printed by the installer:

```sh
./rollback.sh --transaction 20261006T120000Z-1234
```

Rollback restores the exact pre-install owned paths, account databases, and
service enable/active state captured by the package installer. Packages added
by Ansible as host prerequisites remain installed; removing shared OS packages
automatically could damage other applications on the server.

## SSH username/password authentication

Generate a protected bcrypt hash file using the new package utility:

```sh
./payload/bin/safeterm-password-hash -output /secure/alice.hash
```

Set `safeterm_bootstrap_ssh_password_hash_file: /secure/alice.hash` and leave
`safeterm_bootstrap_ssh_public_key_file: ""` for password-only fresh installation.
To update an existing identity, define the complete managed `safeterm_ssh_users`
list with `user_id`, `roles`, and `password_hash`. The hash may be read with a
controller-side file lookup. `authorized_keys` is optional; omit it to disable
key login for that user. Password-bearing template tasks suppress logs and diffs.
Existing identity files are preserved during package upgrades, so bootstrap
options alone do not change an existing user's authentication methods.
See [SSH authentication](../../docs/ssh-authentication.md).
