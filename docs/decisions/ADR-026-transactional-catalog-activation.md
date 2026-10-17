# ADR-026: Transactional catalog activation through termctl

## Status

Accepted. Extends ADR-012's signed catalog and ADR-023's administration lifecycle.

## Context

Adding an approved log alias changes signed catalog content. Requiring manual
signing, file replacement and coordinated restarts is error-prone. Signing live
files alone also lacks a trustworthy rollback baseline after direct manual edits.

## Decision

Add root-only `termctl catalog apply` with local-existing-key and externally
signed input modes, dry-run validation, exact sudoers regeneration, coordinated
service activation and automatic recovery. Default local input is the active
catalog so existing operator workflows can be repaired; --source supports safer
work on a copy. Do not mint a new key, rotate trust, refresh executable hashes
implicitly, import identities, or expand profile/unit boundaries.

Seed a verified checkpoint during installation. Preserve one previous catalog
and a fsynced pending transaction before stopping consumers. If live JSON has a
stale signature, use the checkpoint as the recovery baseline. Refuse activation
if no verified baseline exists. Leave Audit running, preserve inactive-service
state and restart only previously active catalog consumers. Apply installed
preflight and bounded readiness checks. Identical valid content is a no-op.

Serialize with the package lifecycle lock, including inherited installer lock
handling for the checkpoint step. On failure, restore catalog/sudoers and prior
running state. Retain an unrecovered pending journal and require explicit
`termctl catalog rollback` before new lifecycle work. This rollback is independent
of package rollback and never restores account databases or application values.

## Consequences

Ordinary edits need one apply command rather than rebuilding a package. Signature
and executable integrity boundaries remain intact. Activation interrupts active
sessions; no live unsigned reload is introduced. Filesystem log access and log
rotation remain explicit site configuration. ADR-027 supersedes the original
manual-merge upgrade workflow: normal package updates now preserve the verified
installed catalog; portable export and explicit target hash refresh support reuse. Pending-state recovery cannot repair an
unavailable disk, an externally broken service, or already lost unsigned history.

See [catalog activation](../catalog-activation.md) for commands, bounds and limits.
