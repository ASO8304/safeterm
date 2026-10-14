# termctl

The installer places Bash completion at
`/usr/share/bash-completion/completions/termctl`. With the distribution's
bash-completion integration enabled, it loads in a new interactive Bash session.
For the current session, or without that integration, load it explicitly:

```sh
source /usr/share/bash-completion/completions/termctl
```

Use Tab after `termctl service ` for actions, after `termctl service restart `
for SafeTerm service names, or after `termctl user add ` for identity types.
This is Bash administration completion; the restricted SSH terminal has its
own authorization-aware completion. Older releases with incorrectly escaped
generated completion need the corrected binary before regeneration.

Lifecycle administration (run from an ordinary administrator SSH/console,
outside the restricted SafeTerm terminal):

```sh
sudo termctl update /path/to/extracted/safeterm-offline-amd64 --dry-run
sudo termctl update /path/to/extracted/safeterm-offline-amd64
sudo termctl rollback
sudo termctl uninstall
```

Update uses an already existing local signing key. With offline signing, add
`--signed-catalog /path/to/signed-catalog` instead. Download and verify the
release archive checksum before extracting it. The installer verifies its
internal manifest, preserves identities and managed settings, and restarts
services. It retains one pre-install snapshot after success; failed updates
retain their recovery snapshot. Rollback automatically selects the latest
snapshot, including after a failed installation, without any saved ID.
Rollback is explicit, not automatic on installation failure.

Uninstall removes runtime binaries, units and the service-manager sudoers rule.
It retains configuration, accounts, keys, audit data and snapshots; it is
idempotent and supports reinstall without enrollment. It does not implement
data purging. After uninstall, recovery remains available through
`sudo /usr/local/sbin/safeterm-install-rollback` or the package copy
`sudo ./payload/rollback.sh`. These operations use a shared nonblocking lock to
reject concurrent lifecycle changes.

Rollback restores the account databases captured by the existing installer;
use it before making unrelated account/group changes on the host. It also
discards SafeTerm state changes since that snapshot. It requires local snapshot
storage: administrators do not need to copy or remember transaction IDs.

`termctl` is the single local administration entry point installed with
SafeTerm. It provides catalog and configuration discovery, fixed service
status/lifecycle commands, SSH identity inspection, and access to the full
restricted terminal.

```text
termctl help
termctl catalog list
termctl catalog describe ping
termctl catalog prepare --source ./configs --output /tmp/safeterm-catalog
termctl catalog regenerate-hashes --source ./configs --output /tmp/safeterm-catalog
termctl role list
termctl role describe netops
termctl service status all
termctl service restart gateway
termctl service start all
termctl service stop all
termctl service enable all
termctl service disable ssh
termctl config list
termctl config describe cats-options
termctl user list
termctl shell
```

The optional `--config-dir DIR` prefix selects a configuration tree other than
`/etc/safeterm`; it is useful when preparing or inspecting an offline catalog:

```sh
termctl --config-dir /tmp/safeterm-catalog catalog list
termctl --config-dir /tmp/safeterm-catalog role describe viewer
```

`catalog prepare` and its `regenerate-hashes` alias create an unsigned catalog
with SHA-256 digests refreshed from the executable paths registered in the
source configuration. After review, sign the resulting catalog with the
offline signing tool:

```sh
safeterm-catalog-sign \
  -config /tmp/safeterm-catalog \
  -private-key /secure/safeterm/catalog.key \
  -signature /tmp/safeterm-catalog/catalog.sig
```

`role list` shows every configured role and its operation count. `role describe`
shows the operations authorized by one role. These commands read the signed
deployment configuration and do not change authorization.

SSH identities can be added, updated, or removed without editing JSON by
hand. When an SSH user is added without an authorized key or a hash file,
`termctl` prompts for the password twice without echo, validates its complexity,
generates a bcrypt hash, and stores only the hash. A hash file remains available
for unattended provisioning:

```sh
termctl user add ssh --id alice --roles viewer,netops
# New password: (hidden)
# Confirm password: (hidden)

termctl user add ssh --id alice --roles viewer,netops \
  --password-hash-file /secure/alice.hash \
  --authorized-key-file /secure/alice.pub
termctl user auth ssh --id alice --password-hash-file /secure/alice.hash
termctl user del ssh --id alice
termctl user password ssh --id alice
```

Console users are internal SafeTerm identities. They all enter through the one
restricted Linux `safeterm` account, then authenticate with their own username
and password. No per-person Linux account, `useradd`, PAM change or `chpasswd`
is involved. Console and SSH user lists are independent, even for the same ID.

```sh
sudo termctl user add console --id alice --roles viewer,netops,config-viewer
# New password: (hidden)
# Confirm password: (hidden)
sudo termctl user auth console --id alice --roles viewer,config-viewer
sudo termctl user password console --id alice
sudo termctl user del console --id alice
sudo termctl user list
sudo -iu safeterm
# Username: alice
# Password: (hidden)
```

For unattended provisioning, add `--password-hash-file /secure/alice.hash`.
`--os-user` is obsolete for Console users. Console changes take effect without a
service restart and invalidate existing sessions when credentials or roles change.
The password command prompts twice without echo. Passwords must be 8–72 printable
ASCII characters, contain uppercase and lowercase letters and a number. Only
bcrypt hashes are stored. This policy applies to both SSH and internal Console
password entry; a precomputed hash cannot prove the original password's complexity.
An explicit password reset also enables a disabled Console identity. A roles-only
update keeps a disabled identity disabled.

After upgrading an old OS-user mapping, provision its internal password once:

```sh
sudo termctl user password console --id safeterm
```

Legacy entries without a hash remain disabled. The Linux account password is
separate and unchanged; `sudo passwd safeterm` is only needed if the site permits
physical Linux login through that shared account. See
[Console authentication and migration](console-authentication.md).

The command validates roles, password hashes, keys, duplicate identities, and
the rule that at least one valid identity remains in each map. It writes the
root-managed files atomically.

Service lifecycle commands use a fixed map of SafeTerm units. `all` starts and
enables services in dependency order, and stops or disables them in reverse
order. The wrapper never accepts a raw executable path, arbitrary systemd unit,
or shell expression. Generate shell completion with:

```sh
termctl completion bash > ~/.local/share/bash-completion/completions/termctl
termctl completion zsh > ~/.zfunc/_termctl
```

`sudo -u safeterm termctl shell` opens the restricted terminal through the
shared entry account, then prompts for the internal Console username/password.
`execute` and `config` use that authenticated identity.

All service actions require the operating-system permissions needed by
`systemctl`; administrators normally invoke them with `sudo`:

```sh
sudo termctl service status all
sudo termctl service start all
sudo termctl service stop all
sudo termctl service restart ssh
sudo termctl service enable all
sudo termctl service disable config-network
```

The fixed service names are `audit`, `identity`, `gateway`, `ssh`, `diagnostics`,
`network`, `logs`, `services`, `config-application`, `config-database`, and
`config-network`. `termctl service status` without a name is equivalent to
`termctl service status all`.

Configuration discovery is read-only:

```sh
termctl config list
termctl config describe RESOURCE_ID
```

For shell completion, install one generated script for the current user:

```sh
mkdir -p ~/.local/share/bash-completion/completions
termctl completion bash > ~/.local/share/bash-completion/completions/termctl
termctl completion zsh > ~/.zfunc/_termctl
```

Configuration changes always use the proposing user's own preview and y/N
confirmation. Separate approval is removed. Existing registries remain readable:
require_separate_approval is ignored and apply_roles follows propose_roles.
A legacy config-approver role alone no longer grants apply access.

Inside the restricted SSH or local-console terminal, Ctrl+C stops the active
stream and returns to the prompt. Ctrl+] (Control plus right bracket) closes
the entire SafeTerm session, including during an operation or a config prompt.
The input reader cancels outstanding IPC waits; an active stream also receives
semantic termination with a bounded five-second cleanup request. Exiting
`sudo -iu safeterm` returns to the invoking shell; a physical-console login
returns to the login screen, and SSH disconnects. It never opens an OS shell
inside SafeTerm. Cancellation cannot undo a configuration change already
committed by the server.

Identity updates preserve the existing file UID, GID and permissions during
atomic replacement. In particular, SSH users.json must remain
root:safeterm-broker mode 0640, and console users.json root:safeterm-identity mode 0640.
The shared safeterm account must never belong to safeterm-identity.
SSH identities are loaded at Broker startup: restart only `termctl service
restart ssh` (with sudo) after changing SSH users or passwords. Existing
connections will be interrupted.

`sudo termctl service restart all` stops dependents in reverse dependency
order, resets failed/start-limit state for the fixed SafeTerm unit list, then
starts dependencies first. It stops on the first failure. This avoids repeated
implicit Broker starts from sequential restarts of its required services.
Mutating service commands reject non-root invocation with a sudo instruction;
status remains available with ordinary operating-system read permissions.

On an older installation where a user edit changed the SSH file to root:root,
repair with `sudo chown root:safeterm-broker /etc/safeterm/ssh/users.json` and
`sudo chmod 0640 /etc/safeterm/ssh/users.json`. If systemd reports start-limit-hit,
run `sudo systemctl reset-failed safeterm-ssh-broker.service` before starting it.
Upgrade to the corrected termctl before making further identity changes.

## Contextual question-mark help in the restricted terminal

Alongside normal Tab completion, enter a standalone `?` token and press Enter:

```text
?                         Main commands with short descriptions
execute ?                 Authorized operations with descriptions
execute ping ?            Ping parameters, constraints and an example
config ?                  Configuration subcommands with descriptions
config edit ?             Authorized configuration resources
config edit cats-options ? Registered fields for that resource
```

This help is available in both SSH and local-console SafeTerm sessions. It is
read-only, filters operations/resources through the user's permissions, and
returns to the normal prompt. It does not change Linux-style Tab behavior.
The question mark must be a separate token; it is not a wildcard. These are
restricted-terminal commands, not Bash commands or `termctl` CLI subcommands.
