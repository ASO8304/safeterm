# termctl

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

Local console mappings use the operating-system account and a SafeTerm user
id. They control the restricted local console only; web authentication is a
separate identity domain:

```sh
termctl user add console --os-user safeterm --id console \
  --roles viewer,config-viewer
termctl user auth console --os-user safeterm --roles viewer,netops
termctl user del console --os-user safeterm
termctl user password console --os-user safeterm
```

The password command prompts twice without echo. It enforces at least eight
characters, including an uppercase letter, a lowercase letter, and a number;
printable symbols are allowed and control characters are rejected. The SSH password is stored as bcrypt; the console password is passed
to the Linux account password database through `chpasswd` and is never stored
in the SafeTerm identity map.

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

`termctl shell` opens the existing restricted SafeTerm terminal, where `execute`
and `config` workflows use the authenticated local-console identity.

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

The fixed service names are `audit`, `gateway`, `ssh`, `diagnostics`,
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
