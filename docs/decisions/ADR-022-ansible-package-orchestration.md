# ADR-022 — Ansible Orchestrates the Verified Package Installer

## Status

Accepted

## Context

Manual deployment required an administrator to install host packages, transfer
the correct architecture bundle, select a signing workflow, run checks in the
right order, and remember the rollback transaction. Reimplementing installation
inside Ansible would create a second security-sensitive installer and could
drift from checksum, signature, preservation, and rollback behavior already
verified in the release package.

## Decision

Ansible prepares Ubuntu 24.04 and orchestrates the existing offline package
installer. The controller supplies the application archive; the role verifies
its SHA-256 after transfer, invokes package dry-run, and then invokes the same
transactional installer used manually. Local and offline signing remain
explicit modes.

Dependency management supports normal APT sources, an administrator-provided
APT repository, and a preinstalled mode that performs no package-network
operation. This lets an isolated server receive both application and operating
system dependencies from controlled internal sources.

The role owns identity maps and the Config Manager registry only when their
variables are non-empty. Package installation is keyed by a protected release
fingerprint, while configuration enforcement and full preflight run on every
play. Rollback delegates to the installed transaction restore command. Ansible
does not remove shared operating-system packages during rollback.

## Consequences

- Installation and upgrades use one controller command and keep a single
  implementation of package verification and rollback semantics.
- Internet access is unnecessary on a pre-provisioned target or when an internal
  APT repository is configured.
- Identical playbook runs do not create needless rollback transactions.
- Production still requires the signed catalog artifact to be prepared before
  deployment; the playbook cannot hold or impersonate an offline signing key.
- Full host rollback restores SafeTerm-owned state. Dependency packages remain
  because they may be shared by unrelated workloads.
