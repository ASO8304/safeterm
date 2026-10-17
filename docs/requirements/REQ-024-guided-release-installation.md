# REQ-024: One-command downloaded Release installation

**Source:** Request to install using one script without multiple setup commands.
**Priority:** High
**Status:** Implemented

## Acceptance criteria

- Extracted Release supports `sudo ./install.sh` for guided fresh install and existing-host update.
- Verify package integrity and architecture before executing bundled helper code.
- Detect missing OS prerequisites; interactive setup can provision through APT.
- Offline failure gives actionable guidance; dry run never installs packages.
- Ask for internal username, hidden password and confirmation; enforce existing complexity policy.
- Persist bcrypt hashes only; temporary hash files are private and cleaned up.
- Guided initial identity can use both SSH and Console without separate Linux accounts.
- Existing users/passwords/catalog/managed values remain intact during update.
- Explicit hash/key, offline signing and automation flags remain supported.
- Noninteractive credential failure cannot hang awaiting input; APT needs explicit opt-in without a terminal.
- Installation guide leads with one command and explains independent credentials.

## Verification

`tests/e2e/installer_enrollment_test.go` tests actual shell enrollment orchestration,
default/custom/invalid identities, provided credentials, nonterminal/noninteractive
failure, cancellation and prerequisite policy with isolated package-manager stubs.
`cmd/password-hash/terminal_linux_test.go` drives real pseudoterminals to verify
passwords are not echoed, confirmation is required, invalid passwords do not persist
hashes, and successful hashes have mode 0600.

Related decision: [ADR-028](../decisions/ADR-028-guided-release-installation.md).
