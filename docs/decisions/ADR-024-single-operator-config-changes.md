# ADR-024: Single-operator configuration changes

Status: Accepted. Supersedes the separate-approval behavior of ADR-014.

## Context

Requiring a second identity for ordinary configuration changes complicates
operation across hosts. Operators need to preview and commit their own changes.

## Decision

Remove separate-identity approval from the manager and restricted shell.
Keep typed validation, preview, local y/N confirmation, short-lived single-use
tokens, creator binding, concurrency checks, audit, backups and rollback.

For writable resources, propose_roles determines both proposal and apply
authorization. Normalize legacy apply_roles to propose_roles when loading the
registry. Accept but ignore legacy require_separate_approval so installed
registries do not require manual migration. Keep --single-operator as an
installer compatibility no-op. Existing config-approver catalog roles remain
parseable, but do not authorize applying other identities' proposals.

Read, activation and rollback permissions remain independent. If activation
is configured, its authorization still applies to the proposing user.

## Consequences

Existing installations change behavior after upgrading the runtime, including
registries that previously requested independent approval. There is no need to
edit preserved resource registries or re-sign the catalog for this transition.
An identity granted only legacy apply permissions loses that permission;
administrators must assign a propose role for editing. This is intentional.
