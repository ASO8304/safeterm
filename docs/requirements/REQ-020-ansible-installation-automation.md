# REQ-020 — Ansible Installation Automation

**Source:** M20 — Automated Server Installation
**Status:** Implemented in M20

## Requirement

An administrator shall be able to install or upgrade SafeTerm on Ubuntu 24.04
with one controller-side command. Automation shall provision required host
dependencies, preserve the signed-package security boundary, support targets
without Internet access, manage explicitly selected configuration files, verify
the completed system, and expose transactional rollback.

## Acceptance criteria

- [x] A POSIX wrapper invokes the complete Ansible installation with one command.
- [x] Python is bootstrapped before fact gathering and missing target packages
  are installed through APT in online or administrator-defined repository mode.
- [x] Preinstalled mode performs no APT network operation and fails when a
  required executable is absent.
- [x] The release archive is sourced from the controller and both controller and
  target SHA-256 values are compared before extraction.
- [x] Local demo signing and production offline-signed catalog inputs are
  explicit, mutually exclusive deployment modes.
- [x] The package installer dry-run succeeds before any transactional install or
  upgrade is attempted.
- [x] Re-running an identical deployment skips package mutation using a protected
  deployment fingerprint while still applying managed configuration and checks.
- [x] SSH identities, console identities, and the Config Manager registry are
  managed only when explicitly supplied and are JSON-validated before replacement.
- [x] Installed preflight and active-state checks cover every SafeTerm service.
- [x] A rollback playbook and one-command wrapper invoke the package transaction
  restore path by latest or explicit transaction ID.
- [x] Static regression tests cover playbook structure, dependency modes,
  package verification, dry-run ordering, preflight, idempotence, and rollback.
