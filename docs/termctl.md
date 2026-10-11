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

The command accepts only catalog-owned operation names and a fixed service
name map. It never accepts a raw executable path, arbitrary systemd unit, or
shell expression. Generate shell completion with:

```sh
termctl completion bash > ~/.local/share/bash-completion/completions/termctl
termctl completion zsh > ~/.zfunc/_termctl
```

`termctl shell` opens the existing restricted SafeTerm terminal, where `run`
and `config` workflows use the authenticated local-console identity.
