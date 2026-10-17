# ADR-028: One-command guided Release installation

## Status

Accepted. Extends the offline package installer and ADR-027 configuration preservation.

## Context

Separate prerequisite installation, password hashing, signing flags and Console
enrollment make the ordinary downloaded-Release installation unnecessarily hard.
Existing automation must retain a deterministic, noninteractive interface.

## Decision

After extracting a Release, `sudo ./install.sh` performs guided fresh installation
in a terminal. Verify package checksums and architecture before loading installer
support code. Require root and systemd as PID 1 before OS/deployment mutation.
Detect required tools and install missing Ubuntu/Debian prerequisites through
configured APT repositories. Use `--no-upgrade` for already installed requested
packages. Air-gapped targets must provide their own package media/internal mirror.

Use local signing by default only for interactive fresh installation without an
explicit signing mode. Ask for a SafeTerm username (default admin) and a hidden,
confirmed, complexity-checked password using the existing password-hash binary.
Keep only a mode-0600 bcrypt hash in the private installer workspace; remove it
on normal completion, failure and handled termination signals. Never put plaintext
passwords in shell variables, arguments or JSON. Enroll the selected internal ID
in both independent SSH and Console stores on a guided fresh install. Existing
stores, credentials and installed catalog are preserved on update.

Retain explicit hash/public-key bootstrap and offline signing. `--non-interactive`
disables enrollment prompts; `--install-dependencies` explicitly allows APT without
a terminal. A dry run never installs OS packages or changes live deployment state.
All existing bundle validation and package rollback mechanisms remain in place.

## Consequences

The common installation has one command after extraction. The initial SSH and
Console password is the same, but later password changes are independent. No new
Linux account is created per SafeTerm identity. Explicit unattended/key-only
installation retains separate Console enrollment. Failure to reach APT or provide
credentials exits with actionable guidance. OS package installation is not covered
by SafeTerm's deployment rollback; missing prerequisites are provisioned before
the SafeTerm transaction. Physical entry-account login remains a separate OS
administration choice.
