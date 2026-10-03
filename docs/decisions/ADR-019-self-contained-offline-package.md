# ADR-019 — Self-Contained Offline Installation Package

## Status

Accepted

## Context

Production and demonstration hosts may have no Internet access. Copying the Git
checkout alone still requires a Go toolchain and cached modules, while building
the signed catalog on another host pins the wrong system executable hashes.
Repeated demo installation also rotated the SSH host key and could reset managed
configuration.

## Decision

Build a platform-specific directory on a connected Linux machine. It contains
static SafeTerm service binaries, catalog preparation/signing tools, catalog
sources, managed seeds, schemas, units, the base installer, a target installer,
and a checksum manifest. The target needs neither source code nor Go.

Catalog preparation remains target-local because executable integrity binds to
the exact target files. Production uses a two-stage process: prepare on target,
sign on the protected signing host, and return only the signed catalog. A
separate explicit local-signing mode supports evaluation and retains its key in
a root-only directory while warning that this weakens the signing boundary.

Upgrades reuse the installed SSH host key and identity/registry files. Managed
configuration is copied with metadata before the base installer runs without
starting services, restored before service startup, and checked again. Package
checksums and bundle preflight precede persistent mutation.

## Consequences

- Air-gapped targets require no Git, Go, or network download.
- AMD64 and ARM64 packages are different artifacts and fail closed on an
  architecture mismatch.
- Distribution-level prerequisites remain the operator's responsibility and
  must come from Ubuntu media or an internal package mirror.
- Production upgrades still require the offline catalog signing key whenever
  target executable hashes change.
- Full pre-install state restoration is defined by
  [ADR-020](ADR-020-transactional-installation-rollback.md).
