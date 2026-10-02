# REQ-015: End-to-End Deployment and Demo

**Source:** M15 — End-to-End Deployment and Demo
**Priority:** Critical
**Status:** Implemented in M15

## Description
SafeTerm must be deployable on a fresh Linux systemd host from a complete,
signed, checksum-protected bundle. Deployment must fail before changing the
host when the catalog, executable hashes, SSH material, resource registry,
managed-file inputs, binaries, or service units are incomplete or unsafe.

The milestone must also provide a reproducible demonstration that crosses the
real public and local boundaries: SSH public-key authentication, the restricted
terminal, peer-authenticated Gateway routing, a capability-scoped Runner,
integrity-verified direct execution, and durable correlated audit records. CI
must exercise the same data path without requiring root or systemd, while the
operator demo runs the separately deployed service processes on a disposable
host.

M15 supports fresh installation on a dedicated Linux host. In-place upgrade,
distribution packaging, high availability, external audit checkpoints, and
production PKI lifecycle automation remain separate release-engineering work.

## Acceptance Criteria
- [x] **AC-1:** `catalog-prepare` copies the declarative catalog into a new
  directory and replaces every executable digest with the SHA-256 of the exact
  target-host executable without generating or copying private signing keys.
- [x] **AC-2:** An offline signing step produces the detached catalog signature
  and public trust anchor; the private key is absent from the runtime bundle.
- [x] **AC-3:** The bundle builder compiles all runtime binaries, includes the
  signed catalog, SSH identity and host material, resource registry, managed
  seed files, systemd units, and a deterministic SHA-256 manifest.
- [x] **AC-4:** Preflight rejects missing, linked, writable, unsigned, tampered,
  or inconsistent bundle content and verifies executable digests and formats,
  SSH material, resources, managed inputs, and unit hardening before install.
- [x] **AC-5:** The installer supports a non-mutating dry run, verifies the
  checksum manifest and preflight before mutation, creates dedicated accounts
  and groups, applies least-privilege ownership/modes, and installs all runtime
  artifacts at the paths used by the shipped units.
- [x] **AC-6:** Installed preflight validates the root-owned runtime catalog and
  registry, service accounts, managed files, executables, SSH material, and
  systemd units before any service is started.
- [x] **AC-7:** Service startup follows Audit → Runners → Gateway → Config
  Managers → SSH Broker and fails the installer unless every unit is active.
- [x] **AC-8:** The automated E2E test authenticates a real SSH client, obtains
  the restricted PTY shell, routes `system_info` through real Unix sockets and
  the Gateway/Runner pipeline, executes the hash-verified `/usr/bin/uname`
  binary without a shell, and observes a successful response.
- [x] **AC-9:** The E2E test verifies four durable, request-correlated Gateway
  and Runner audit events for the demonstrated operation.
- [x] **AC-10:** The E2E test proves raw SSH exec remains rejected and cannot
  bypass the restricted shell.
- [x] **AC-11:** The root-only demo generates disposable catalog and SSH keys,
  builds and installs a fresh bundle, verifies the Broker host key, demonstrates
  identity, catalog, execution, and configuration listing, and reports PASS.
- [x] **AC-12:** Operator documentation includes prerequisites, offline signing,
  dry run, installation, demo, verification, troubleshooting, and the explicit
  fresh-host scope. CI and milestone evidence retain the successful checks.

## Related Tests
- TEST-DEPLOY-001 through TEST-DEPLOY-004
- TEST-E2E-001 through TEST-E2E-006

## Related ADRs
- ADR-017
