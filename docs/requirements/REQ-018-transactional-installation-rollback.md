# REQ-018 — Transactional Installation Rollback

**Source:** M18 — Transactional Installation Rollback
**Status:** Implemented in M18

## Requirement

Every persistent package installation shall capture a verified pre-install
transaction and provide an offline rollback command that restores all host
state owned or modified by the SafeTerm installer.

## Acceptance criteria

- [x] **AC-1:** Dry-run creates no transaction; a real install creates one
  before the local signing key or any other persistent installation artifact.
- [x] **AC-2:** The snapshot includes existing SafeTerm binaries, configuration,
  managed and audit data, units, home directory, local signing keys, and the
  account databases and sparse account logs modified by user and group creation.
- [x] **AC-3:** The transaction records the enabled and active state of every
  SafeTerm systemd unit before installation.
- [x] **AC-4:** A SHA-256 manifest covers the archive and all restoration
  metadata, and rollback refuses mutation when verification fails.
- [x] **AC-5:** Rollback stops and disables current services, removes the entire
  current installer-owned footprint, and extracts the previous snapshot with
  ownership and modes preserved.
- [x] **AC-6:** Rollback restores previous unit enablement, masking, and active
  states after a daemon reload.
- [x] **AC-7:** Fresh-install rollback removes the accounts, home, local key,
  configuration, data, binaries, and units introduced by installation.
- [x] **AC-8:** The rollback executable is both embedded in the offline package
  and installed at a stable root-only administration path.
- [x] **AC-9:** A successful rollback consumes its transaction and safely moves
  the `latest` pointer to the newest remaining transaction.
- [x] **AC-10:** Shell syntax and deployment-asset tests enforce snapshot,
  integrity, cleanup, systemd restoration, and package-completeness markers.

## Evidence

- `TEST-PACKAGE-001` verifies the builder, installer, and rollback invariants.
- `make verify` validates shell syntax on Linux and runs the integration suite.
