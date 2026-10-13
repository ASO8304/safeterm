# ADR-023: Local installation lifecycle through termctl

Status: Accepted

## Context

Fleet operators need a stable command interface without tracking per-host
transaction identifiers. Repeated upgrades must not retain unlimited snapshots.

## Decision

Expose update, rollback and uninstall through termctl. Update accepts an
extracted offline package and delegates to its checksum-verifying installer.
Use local signing only when the existing default signing key is available;
otherwise require an explicitly signed catalog. Keep the latest pre-install
snapshot after a successful upgrade. Keep recovery state on failures.

Uninstall removes the runtime and preserves state, accounts, keys and audit
history. Reinstall uses the existing identity maps. Retain the standalone
rollback and uninstall helpers after runtime removal. Serialize all package
lifecycle operations through flock on a fixed /run/lock path.

## Consequences

No transaction ID is required for normal recovery. Snapshots still require
disk space. Uninstall is not data purging. Existing rollback restores global
account databases; unrelated account changes must not occur before rollback.
Fleet package distribution and signature custody remain external to termctl.
