# ADR-030: Contextual terminal and atomic configuration drafts

## Status

Accepted. Extends ADR-014, ADR-024 and ADR-029.

## Context

Repeated resource/section/key coordinates and visible proposal tokens make
routine configuration work difficult. Flat service operation IDs clutter the
menu. Simplification must retain registered-field, role, version, audit and
privileged-writer boundaries and preserve application files.

## Decision

Use a resource/section editor shared by SSH and Console: config RESOURCE,
edit SECTION, show, set KEY VALUE, diff, next, end, cancel and rollback.
Stage an in-memory resource draft; end creates and applies an internal,
creator-bound proposal. No per-field confirmation or exposed token is required.
Next only changes context; end commits the entire resource once. Exit,
disconnect, cancellation and the emergency shortcut discard unsaved drafts.

Extend semantic Patch with an optional bounded list of FieldChange entries.
Reject mixed single/batch forms, duplicates, unknown/readonly fields and
excessive batches. Normalize registered fields, validate the final document,
render/reparse, and compare each normalized value. The manager and root writer
share that renderer and retain independent version/authorization/profile checks.
Apply uses the existing one-write/backup/activation/recovery path for the full
batch, rather than chaining separate field commits. Protocol envelopes remain
version 1; older services reject the new unknown property, so upgrade services
and brokers together using the package lifecycle.

Bound drafts to 512 fields and 256 KiB of values/coordinates. Hide secret input,
redact reads/diffs and omit set command lines from history. Diff is a read-only
local draft view; final server validation is at end. A stale end retains the
draft and requires reopening; there is no force overwrite.

Present a minimal root menu and contextual ?/Tab. Map readable command paths
such as execute service status ssh to the original signed operation IDs. Keep
permission filtering and gateway authorization. Preserve legacy syntax for
scripts but hide redundant root menu entries from normal discovery. Root
termctl deployment administration remains outside the restricted terminal.

## Consequences

Administrators can update several fields/sections in one transaction, including
coordinated cross-field changes. Existing ownership/comment/ACL and audit
controls continue to apply. A session is tied to one resource version; concurrent
external updates require reviewing/reopening. This deliberately borrows context
navigation, not FortiGate's next/save semantics. No arbitrary shell, file paths,
key creation, privilege broadening or catalog schema migration is introduced.
Optional field descriptions improve contextual help without changing validation.

## Verification

Tests cover cross-section drafts, repeated set, cancellation, stale versions,
secret redaction/history, range and cross-field validation, one backup per batch,
whole-file rollback and denied fields. Writer tests include semantic batch
commits and preservation, with actual root/non-root integration in CI. Existing
unit, deployment E2E, adversarial and race suites remain required.
