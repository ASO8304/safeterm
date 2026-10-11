# termctl

`termctl` is the single local administration entry point installed with
SafeTerm. It provides catalog and configuration discovery, fixed service
status/lifecycle commands, SSH identity inspection, and access to the full
restricted terminal.

```text
termctl catalog list
termctl catalog describe ping
termctl service status all
termctl service restart gateway
termctl config list
termctl config describe cats-options
termctl user list
termctl shell
```

SSH identities can be added, updated, or removed without editing JSON by
hand. Passwords are supplied through a bcrypt hash file, never as a command
line argument:

```sh
termctl user add ssh --id alice --roles viewer,netops \
  --password-hash-file /secure/alice.hash \
  --authorized-key-file /secure/alice.pub
termctl user auth ssh --id alice --password-hash-file /secure/alice.hash
termctl user del ssh --id alice
```

Local console mappings use the operating-system account and a SafeTerm user
id. They control the restricted local console only; web authentication is a
separate identity domain:

```sh
termctl user add console --os-user safeterm --id console \
  --roles viewer,config-viewer
termctl user auth console --os-user safeterm --roles viewer,netops
termctl user del console --os-user safeterm
```

The command validates roles, password hashes, keys, duplicate identities, and
the rule that at least one valid identity remains in each map. It writes the
root-managed files atomically.

The command accepts only catalog-owned operation names and a fixed service
name map. It never accepts a raw executable path, arbitrary systemd unit, or
shell expression. Generate shell completion with:

```sh
termctl completion bash > ~/.local/share/bash-completion/completions/termctl
termctl completion zsh > ~/.zfunc/_termctl
```

`termctl shell` opens the existing restricted SafeTerm terminal, where `run`
and `config` workflows use the authenticated local-console identity.
