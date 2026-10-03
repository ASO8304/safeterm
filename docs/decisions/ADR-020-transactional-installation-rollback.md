# ADR-020 — Transactional Installation Rollback

## Status

Accepted

## Context

Preserving selected files during an upgrade prevents common data loss, but it
does not undo a failed or unwanted installation. SafeTerm installation changes
binaries, configuration, managed and audit data, systemd units and enablement,
local accounts, a login home, and optionally a catalog signing key. An operator
needs one offline command that returns those changes to the state observed
immediately before installation.

## Decision

The package installer creates a root-only transaction after all non-mutating
package and bundle verification succeeds and before the first persistent
change. The transaction contains a tar archive of every existing path that the
installer owns, the system account databases that the base installer may edit,
an explicit replacement-path list, and the enabled/active state of all SafeTerm
units. A SHA-256 manifest covers the archive and metadata.

Rollback first verifies that manifest. It then stops and disables the current
services, deletes only allowlisted installer-owned paths, restores the archived
paths with their metadata, reloads systemd, and restores prior enablement,
masking, and active state. Restoring the account databases removes identities
created by a fresh installation and recreates any prior identities exactly.
The transaction is consumed only after successful restoration.

Installation journal records are retained because they are host audit evidence,
not SafeTerm-owned mutable state. Operators run rollback in a maintenance window
because exact account-database restoration intentionally supersedes unrelated
account changes made after the snapshot.

## Consequences

- Fresh installation and upgrade now share the same rollback mechanism.
- Rollback works without Internet access, Git, Go, or the original source tree.
- Retained transactions consume disk proportional to the previous SafeTerm
  installation, especially audit and managed data.
- Post-snapshot changes under SafeTerm-owned paths are intentionally discarded.
- Host journal history continues to show both installation and rollback.
