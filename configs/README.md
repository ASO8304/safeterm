# SafeTerm administrator configuration

All SafeTerm administrator settings live under `/etc/safeterm`:

```text
/etc/safeterm/
  access/
    ssh/                 SSH users and host key
    console/             Console users
    roles.d/             Role definitions
    policies.d/          Exact role-to-operation permissions
  commands/
    operations.d/        Command definitions and arguments
    executables.d/       Approved binary paths and SHA-256 hashes
    profiles.d/          Runner isolation and resource limits
  resources.d/           Application configuration, services and logs
  network/policy.json    Editable interfaces and network roles
  trust/                 Trusted catalog public key
  docs/                  Administration, network and diagnostic guides
```

Copy a directory's inactive `TEMPLATE.json` to a new JSON filename, edit it,
then run `sudo termctl check` and `sudo termctl apply`. Inside SafeTerm, login
shows the command menu, `?` help keeps your input, and `back` discards the item
you are leaving and shows its parent menu. `next` keeps staged item edits.
Manage credentials with
`sudo termctl user`; preserve their existing roles when changing them.

Network policy starts with no editable interfaces. Add exact device names to
`network/policy.json` as root. Reopen the SafeTerm network editor after changing
policy. `end` saves a network candidate; `execute network apply` starts a timed
trial, and `execute network confirm` makes it persistent. See `docs/network.md`.

Generated runtime data lives under `/var/lib/safeterm/generated`; verified package
defaults live under `/usr/share/safeterm/defaults`. Inspect defaults using
`termctl defaults`. Edit administrator inputs here and apply them through termctl.
Application files stay at their native paths, explicitly registered in resources.d.
No `/etc/safeterm-managed` directory is needed.

Package updates migrate the v0.27 flat source layout into these directories while
preserving files and metadata. Conflicting old/new paths stop migration. Package
rollback restores the saved configuration and matching binaries together.

See `docs/administration.md` and each definition directory's README for examples.
