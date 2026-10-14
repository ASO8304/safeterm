# ADR-025: Internal Console users behind one restricted Linux entry account

## Status

Accepted. Supersedes ADR-018's Console OS-user mapping. Its shared terminal UX
and the independent SSH authentication architecture remain in effect.

## Context

Creating one Linux account for every Console operator couples product identity
to host administration. Merely adding a login prompt under one shared UID would
be unsafe: that UID could bypass the prompt and send arbitrary users/roles to
Gateway and Config Managers. Password hashes also cannot be readable by every
process of the shared entry account.

## Decision

Use one unprivileged `safeterm` Linux entry account with the fixed Console login
shell and mandatory internal username/password authentication. Store bcrypt
hashes and roles in a root-managed, Identity-only credential file. Add a separate
non-root `safeterm-identity` service, with no execution capability, to authenticate
users and issue random, in-memory, process-bound sessions with absolute expiry.

Gateway and Config Managers verify each Console session with Identity using
kernel UID/PID and derive roles from the authority. Apply the same checks to
unary/streaming/control/config paths; bind config proposals to the login session.
Terminate active operations at session expiry and revalidate streams every 15
seconds for revocation. Keep bounded transactions' existing deadlines. Login
requires durable audit acknowledgement; all login transport, concurrency, bcrypt
work and session storage have bounds. Never log plaintext credentials or tokens.

Do not modify PAM, create per-person Linux users, copy SSH credentials to Console,
or automatically unlock the Linux entry account. Preserve old records during
upgrade, but disable legacy entries without internal hashes. Provision/reset
internal passwords with `termctl user ... console --id ID`; retire --os-user.

## Alternatives considered

1. Continue per-person Linux accounts: rejected for host provisioning overhead.
2. Shared UID with a local-only password prompt: rejected because UID-based
   Gateway/config trust could bypass authentication and invent roles.
3. Root Console process: rejected because a parser/login flaw would gain host
   privileges. Root is required only for administration of the credential file.
4. Redis/database session store: unnecessary for this single-host product;
   bounded memory is sufficient, and restart intentionally revokes all sessions.
5. Unbound bearer tokens: rejected because another process under the shared UID
   could replay a leaked token without the originating Console process identity.

## Consequences

Operator count no longer affects Linux account count. Audit records retain the
internal user identity. Credential/role changes revoke access without restart;
Identity restart invalidates all sessions. One additional hardened service is
required and is included in package, lifecycle, preflight and Ansible management.
Operators must provision internal passwords once after a legacy upgrade. Existing
Linux users are preserved; no hidden or destructive account migration occurs.

A shared UID still permits denial of service between its processes; do not run
arbitrary programs as that user. Root/kernel compromise remains outside this
boundary. The service adds bcrypt cost at login and small local verification
round trips per request, without Redis or a remote dependency. See the
[Console guide](../console-authentication.md) for provisioning, bounds and tests.
