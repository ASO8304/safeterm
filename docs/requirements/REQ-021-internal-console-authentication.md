# REQ-021: Internal Console authentication and verified sessions

**Source:** Operator request for product users independent of Linux users.
**Priority:** Critical
**Status:** Done

## Acceptance criteria

- [x] One shared restricted Linux entry account; no per-person OS account or PAM edit.
- [x] Mandatory internal password authentication, bcrypt-only storage, hidden entry.
- [x] Isolated non-root authority; credential file unreadable by the entry UID.
- [x] Random sessions bound to kernel UID/PID, bounded lifetime/storage, logout/revocation.
- [x] Gateway unary/stream/control and Config Manager requests verify session and roles.
- [x] Caller role/user forgery and cross-process token replay fail closed.
- [x] Rate/concurrency bounds and audit failure denial; credentials never in audit/history.
- [x] Legacy mappings without hashes denied; explicit password provisioning migration.
- [x] termctl, offline package, rollback/uninstall, systemd, Ansible and docs updated.
- [x] SSH identity domain retained independently and covered by regression tests.

## Related tests

- internal/identity: sessions, roles, legacy denial, rate limits, audit failure, IPC separation.
- cmd/gateway and cmd/config-manager: authoritative roles and forged identity/PID rejection.
- internal/isolation: stream authentication and running-process cancellation at session expiry.
- internal/broker: hidden login, history exclusion, no pre-auth dispatch.
- cmd/termctl: internal users without OS account lookup, hashes, duplicates and migration.
- internal/deployment and tests/e2e: package/public-boundary regression coverage.

## Related ADRs

- [ADR-025](../decisions/ADR-025-internal-console-authentication.md)
