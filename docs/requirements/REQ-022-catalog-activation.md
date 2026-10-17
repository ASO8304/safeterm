# REQ-022: Operator-managed signed catalog activation

**Source:** Request to handle validation, signing and service recovery through termctl.
**Priority:** High
**Status:** Implemented

## Acceptance criteria

- Root-only apply, dry run, source-copy and externally signed input modes.
- Reuse installed trust and an existing local key; reject wrong/missing keys.
- Check schema, pinned executable hashes, user/resource role references and sudoers before stopping services.
- Preserve profile/unit boundaries; no implicit executable hash refresh.
- Installer seeds a verified catalog-only recovery checkpoint.
- Edited live input with stale signature uses that verified checkpoint as baseline.
- Keep users, passwords, managed values, resource registry and trust anchor untouched.
- Preserve inactive services; leave Audit running; coordinate stop/start order.
- Recover on publish/preflight/start failure, retain pending state on recovery failure.
- Catalog rollback recovers interrupted transactions without a signing key.
- Idempotent reapplication preserves rollback history; finite checkpoint retention.
- Bash/Zsh completion, administration docs and installation lifecycle integration.

## Verification

`cmd/termctl/catalog_manage_linux_test.go` covers edited-live application, no-op,
dry run, rollback, fault injection, wrong keys/hashes/profiles/roles, offline signing,
missing baseline, interrupted recovery and retained journals.
`cmd/termctl/catalog_lock_linux_test.go` verifies exclusion and inherited lock lifetime
through a real subprocess. Bash completion is exercised by the actual shell.

Related decision: [ADR-026](../decisions/ADR-026-transactional-catalog-activation.md).
