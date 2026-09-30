# REQ-010: Runtime Must Verify Catalog and Executable Integrity

**Source:** M10 — Integrity Verification, ADR-009 executable registry and
signed catalog decisions, and ADR-010 pre-execution verification requirement
**Priority:** Critical
**Status:** Done

## Description
SafeTerm must fail closed when an active catalog is not signed by a trusted
catalog authority or when the file selected for execution no longer matches
the approved executable registry entry. Integrity checks apply independently
inside every Runner profile and cover both process operations and the fixed
`systemctl` controller used by the typed service-management backend.

Every executable entry carries an exact lowercase SHA-256 digest. A Runner
verifies the file at startup and immediately before each execution, resolves
symlinks to a stable canonical path, rejects unsafe file types and modes, and
executes the already-open verified inode through Linux `/proc/self/fd`. The
Gateway and every Runner verify a detached Ed25519 signature over a canonical
catalog representation before accepting the snapshot.

## Acceptance Criteria
- [x] **AC-1:** Every executable has an absolute path, a 64-character lowercase
  SHA-256 digest, and at least one allowed profile. A systemd profile references
  an approved controller executable; process profiles cannot define one.
- [x] **AC-2:** Gateway and Runner reject startup when the detached Ed25519
  catalog signature, public trust key, catalog structure, or signed content is
  missing, malformed, or invalid.
- [x] **AC-3:** Runner startup resolves and verifies every executable available
  to its profile. Missing, non-regular, non-executable, group/world-writable,
  non-ELF, or hash-mismatched files prevent construction.
- [x] **AC-4:** Every execution repeats canonical-path, inode, file-type, mode,
  and SHA-256 verification immediately before process creation. Unknown paths
  are rejected without invoking the process Runner.
- [x] **AC-5:** Symlink retargeting and path replacement are detected. After
  verification, execution uses the held file descriptor rather than reopening
  the catalog path, so a rename cannot substitute another inode.
- [x] **AC-6:** Integrity failures return no stdout/stderr, use exit code -1,
  and become a structured `execution_error` at the existing Core execution
  stage without changing Core or its four interfaces.
- [x] **AC-7:** The typed systemd controller is loaded through the same verified
  process boundary and cannot bypass executable integrity checks.
- [x] **AC-8:** Profile-filtered snapshots retain only the process executables
  and controller executable required by that Runner profile.
- [x] **AC-9:** Catalog signatures cover a deterministic deep copy of version,
  operations, executable paths and digests, profiles, roles, and policies.
  Semantically unordered collections have stable ordering while argv template
  order remains unchanged.
- [x] **AC-10:** An offline standard-library tool can generate Ed25519 keys and
  sign a validated catalog. Private keys are never required by runtime services
  and key generation refuses to overwrite existing key files.
- [x] **AC-11:** The shipped example catalog has a valid detached signature and
  its controller/executable metadata passes config validation.
- [x] **AC-12:** Unit and integration tests prove valid execution, catalog and
  binary tamper rejection, symlink defense, profile isolation, systemctl
  coverage, and fail-closed Core behavior.
- [x] **AC-13:** Runtime catalog replacement uses the same verifier as startup;
  failed verification leaves the active snapshot unchanged, and rollback can
  atomically restore the last already-verified snapshot.

## Related Tests
- TEST-INTEGRITY-001 through TEST-INTEGRITY-017

## Related ADRs
- ADR-008 (direct bounded process execution)
- ADR-009 (capability profiles, executable registry, and signed catalogs)
- ADR-010 (verification immediately before execution)
- ADR-012 (detached catalog signatures and pinned executable files)
