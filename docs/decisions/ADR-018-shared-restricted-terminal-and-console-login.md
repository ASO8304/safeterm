# ADR-018 — Shared Restricted Terminal and Console Login

## Status

Accepted

## Context

The byte-at-a-time SSH prompt allowed only append and backspace. Operators need
normal terminal editing and local VM or physical-console access, but SafeTerm
must not turn either transport into Bash, `sh`, an arbitrary PTY, or a route
around the existing least-privilege services.

## Decision

SSH and local console use one restricted `broker.Shell` grammar and line editor.
The editor keeps bounded per-session history and derives Tab candidates from the
principal's authorized catalog and Config Manager views. It never completes an
operation or resource that the principal cannot list, and it does not retain
secret field values.

The local `/usr/local/bin/safeterm-console` executable is the login shell of the
unprivileged `safeterm` account. It accepts no runtime path or role flags. It
loads the signed catalog and a root-managed `/etc/safeterm/console/users.json`
mapping, requires a real terminal, enters raw mode, and connects to the same
authenticated Unix sockets as the SSH Broker.

The transport identity remains explicit. SSH requests use `broker_type=ssh`;
console requests use `broker_type=console`. Gateway binds each value to a
different peer UID. Config Managers accept both peer UIDs and retain their own
role and resource checks. Socket access uses the narrow `safeterm-clients`
group, which does not grant access to the SSH host private key.

## Consequences

- Local console and SSH behavior stay consistent without duplicating policy or
  execution logic.
- Compromise of the console account does not grant an operating-system shell,
  arbitrary roles, paths, executables, or direct Runner access.
- Administrators must set the console password separately with
  `passwd safeterm`; the installer intentionally creates a locked password.
- Command history is session-local and disappears at logout.
