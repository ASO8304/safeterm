# REQ-017 — Offline Installation Package

**Source:** M17 — Online and Air-Gapped Installation Package  
**Status:** Implemented in M17

## Requirement

SafeTerm shall be distributable from an Internet-connected build machine as a
self-contained, integrity-checked installation directory that can prepare and
install the product on an Ubuntu 24.04 host without Git, Go, module downloads,
or Internet connectivity.

## Acceptance criteria

- [x] **AC-1:** A package builder compiles Linux AMD64 or ARM64 runtime and
  catalog tools and includes every required configuration and systemd asset.
- [x] **AC-2:** A package manifest verifies every included file before the
  target installer prepares a catalog or mutates persistent host state.
- [x] **AC-3:** Target catalog preparation hashes executable files on the exact
  target host and requires either an externally signed catalog or an explicit
  local evaluation-signing mode.
- [x] **AC-4:** Production signing keeps the private catalog key off the target;
  the installer accepts only the returned signed catalog.
- [x] **AC-5:** Fresh installation accepts a validated SSH public key and creates
  restricted SSH and console identities without JSON tooling on the target.
- [x] **AC-6:** Upgrade preserves the SSH host key, identity mappings, resource
  registry, and managed configuration contents.
- [x] **AC-7:** Dry-run reaches package, catalog, bundle, and base-installer
  preflight without creating service accounts or installing files.
- [x] **AC-8:** The package checks target CPU architecture and required Ubuntu
  executables and reports missing offline OS prerequisites explicitly.
- [x] **AC-9:** Operators may select separate approval or the documented
  single-operator config workflow.
- [x] **AC-10:** Shell syntax and deployment-asset tests enforce package
  completeness, offline operation, preservation markers, and pre-mutation
  checksum verification.
