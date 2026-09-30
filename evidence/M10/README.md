# M10 Verification Evidence

Milestone M10 adds detached Ed25519 catalog verification and profile-scoped
executable integrity enforcement without changing Agent Core or its stage
interfaces.

## Delivered Controls

- Every executable registry entry carries an exact SHA-256 digest and an
  explicit profile allowlist.
- Gateway and Runner construct verified catalog stores from a separately
  provisioned public key and detached signature.
- Replacement snapshots must pass the same verifier; failed activation is
  atomic, and the last verified snapshot can be restored.
- Runner verifies canonical path, inode, regular/executable mode, native ELF
  type, and SHA-256 at startup and immediately before every execution.
- The process Runner receives an open `/proc/self/fd` path, preventing a rename
  or symlink retarget from substituting a different inode after verification.
- The systemd controller is an explicit signed executable binding and passes
  through the same integrity Executor.
- `cmd/catalog-sign` generates non-overwriting Ed25519 key pairs and creates
  atomic detached signatures using only the Go standard library.

## Traceability

- Requirement: `docs/requirements/REQ-010-integrity-verification.md`
- Design: `docs/decisions/ADR-012-signed-catalog-and-pinned-executables.md`
- Tests: TEST-INTEGRITY-001 through TEST-INTEGRITY-017
- Matrix: `docs/testing/test-matrix.md`

## Verification Artifacts

- `make-verify.log` — full formatting, vet, build, unit, integration, and
  coverage run ending in `SafeTerm Verification: PASS`.
- `race.log` — race-enabled tests for integrity, isolation, and the integration
  package.

## Operational Notes

- The shipped public key and signature are development evidence. A production
  deployment generates its own offline key pair and does not copy the private
  key to managed hosts.
- Executable hashes are deployment-specific and must be recalculated from the
  exact target-host native binaries before signing.
- The runtime is Linux-first and requires `/proc`. Runner units retain
  `ProtectSystem=strict` so approved executable inodes are not writable by the
  service identity.
