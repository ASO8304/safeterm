# REQ-023: Preserve and reuse site configuration

**Source:** Operator request to retain customized operations and log aliases on updates and reuse them across installations.
**Priority:** High
**Status:** Implemented

## Acceptance criteria

- Default updates retain verified site catalog definitions, approved hashes and trust.
- Existing offline-signed catalogs need no signing key for an unchanged update.
- Release defaults remain separate from active policy; no implicit privilege merge.
- Identities, host keys, resource registry and unrelated private files survive updates.
- Existing managed application files and ownership/permissions remain untouched.
- Stage custom resource paths for bundle validation; reject incompatible profiles and broken references before mutation.
- Export definitions without credentials, signatures or signing keys.
- Explicit portable-source deployment hashes target executables and signs with target trust.
- Explicit apply hash refresh supports transactional deployment and rollback; offline signatures cannot be refreshed.
- Package rollback includes the installed release-default reference tree.
- Document source/target workflow and host-specific permissions/resource boundaries.

## Verification

`internal/deployment/upgrade_linux_test.go` covers semantic/signature preservation,
credential-free export, retained hash approval, private file metadata, stale
fragment cleanup, unsafe directory rejection, custom INI staging and incompatible
profiles. `cmd/termctl/catalog_manage_linux_test.go` covers portable log aliases,
target hash refresh, dry-run immutability and rollback under target trust.
Actual Bash completion tests cover the new commands and options.

Related decision: [ADR-027](../decisions/ADR-027-preserved-site-catalog-and-portable-definitions.md).
