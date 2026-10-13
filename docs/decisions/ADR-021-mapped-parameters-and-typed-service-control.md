# ADR-021 — Mapped Parameters and Typed Service Control

## Status

Accepted

## Context

Log paths must be configurable by administrators but must never become path
input from terminal users. Service status is broadly useful, while lifecycle
and boot-enable changes require stronger authorization. Running unrestricted
sudo or accepting `systemctl UNIT ACTION` parameters would break the catalog
and least-privilege boundaries.

## Decision

A string parameter may carry `value_map`, whose keys are stable user-facing
aliases and whose values are trusted catalog-owned argv tokens. Validation
accepts only a key; translation substitutes the mapped value after policy.
The log operation uses this mechanism with a dedicated log-reader OS identity,
Runner socket, systemd sandbox, and bounded streaming session.

Systemd operations remain argument-free exact unit/action pairs. The
service-manager controller invokes non-interactive sudo with a fixed systemctl
prefix. Deployment derives a sudoers command alias only from signed typed
operations, validates it with `visudo`, and installs it mode 0440. There are no
wildcards. The Runner needs set-user-ID transitions for sudo, so its unit alone
sets `NoNewPrivileges=no` and `RestrictSUIDSGID=no`; all other sandbox controls,
resource limits, executable integrity checks, peer authentication, policy, and
durable audit remain active.

Deployment correction: the service-manager capability bounding set permits
only CAP_SETUID, CAP_SETGID and CAP_AUDIT_WRITE. Ambient capabilities remain
empty: the Runner receives no ambient privilege; setuid sudo can obtain only
the bounded capabilities needed for identity transitions and native Linux
audit messages. AF_UNIX and AF_NETLINK permit local systemd and audit IPC.
An empty bounding set prevented sudo from setting its root GID even with
NoNewPrivileges disabled. Other service profiles retain their existing bounds.

Read-only status and mutating lifecycle operations use separate catalog roles.
Fresh identities receive `service-viewer`, never `service-operator`.

## Consequences

- Administrators can add log aliases without exposing arbitrary paths.
- A compromised terminal session cannot select a different systemd unit,
  action, executable, or sudo command.
- Adding or removing a typed service operation also changes generated sudoers
  policy and therefore requires the signed installation workflow.
- Log file rotation must preserve the group-readable access configured for the
  log-reader service account.
