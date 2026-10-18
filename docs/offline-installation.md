# Offline Release installation

## Install from a downloaded GitHub Release

This source includes a breaking configuration layout change (ADR-032). Use a
Release that explicitly contains it. Old-layout installations are refused before
deployment; preserve a backup and provision a clean installation with new JSON
resource definitions. Do not copy the old configuration tree over the new one.

Download the AMD64 or ARM64 offline archive and its checksum from
[GitHub Releases](https://github.com/ASO8304/safeterm/releases), verify the published
checksum, extract, then run the included script:

```sh
sha256sum -c safeterm-offline-amd64.tar.gz.sha256
tar -xzf safeterm-offline-amd64.tar.gz
cd safeterm-offline-amd64
sudo ./install.sh
```

Use the corresponding arm64 filename on ARM. The guided installer checks OS
dependencies, asks for the initial SafeTerm username/password and selects local
catalog signing. Only the protected key under /root/.config/safeterm can sign.
The initial username is created in both internal SSH and Console identity maps;
it is not a Linux user. Passwords are stored as hashes. Offline signing is
available through --prepare-catalog DIR and --signed-catalog DIR.

```sh
sudo termctl service status all
ssh -tt -p 2222 admin@SERVER_IP
sudo -iu safeterm
```

The Console entry prompts for your internal username and password. Add independent
users with `sudo termctl user add console --id alice --roles viewer,netops` or
`sudo termctl user add ssh --id alice --roles viewer,netops`. SSH map changes
need `sudo termctl service restart ssh`. Do not grant service account shells or sudo.

## Configure and update

```sh
sudo cp /etc/safeterm/resources.d/TEMPLATE.json /etc/safeterm/resources.d/myapp.json
sudo nano /etc/safeterm/resources.d/myapp.json
sudo termctl check
sudo termctl apply
sudo termctl update /path/to/new/extracted/package
```

Define service/config/log blocks independently. The [resource reference](../configs/resources.d/README.md)
explains roles, fields, file permissions, signing and examples. Source JSON,
credentials and trust stay under /etc/safeterm. Generated runtime data lives under
/var/lib/safeterm/generated and must never be edited. Package built-ins are under
/usr/share/safeterm/defaults. Updates preserve new-format custom sources and
identities and rebuild generated data; they require a matching signing method.
Offline operators must supply exact matching signatures; an unsigned changed
candidate fails rather than importing prior generated state.

`sudo termctl apply --rollback` restores a completed definition activation;
`sudo termctl apply --recover` recovers an interrupted one. Neither changes source
JSON or application values. `sudo termctl rollback` is separate package rollback,
using the automatically retained installation snapshot. `sudo termctl uninstall`
removes runtime software and retains configuration/data/snapshots. Package
rollback also restores account databases, so avoid unrelated account changes
before using it. Keep ordinary administrator access for recovery.

Inside SafeTerm, use `?` Enter or Tab to discover authorized commands. Configuration
editing is `config myapp`, `edit SECTION`, `set KEY VALUE`, `diff`, `end`.
Ctrl+C stops an operation and Ctrl+] closes the entire SafeTerm session.
See [terminal](restricted-terminal.md) and [termctl](termctl.md).


## Unattended installation

Use `sudo ./install.sh --local-signing --non-interactive --ssh-user admin
--ssh-password-hash-file /secure/admin.hash` as one shell command. An optional
`--ssh-public-key FILE` enables key authentication. `--dry-run` checks the base
plan without installing. `--no-start` stages runtime without starting consumers;
activation is required before use. Private signing keys are never copied from
another host by default.

Build reproducible packages with `make offline-package OUTPUT=/tmp/package
ARCH=amd64` (or arm64). Go and module dependencies are needed only on the build
machine, never on the target. The target verifies PACKAGE-SHA256SUMS before
privileged deployment. Sign/verify downloaded release checksums through your
trusted release-distribution channel.
