# ADR-027: Preserve site configuration across package updates

## Status

Accepted. Extends ADR-026 and the transactional package installation lifecycle.

## Context

Rebuilding each update from release defaults discards operator-authored commands
and log aliases. Temporarily installing sample application files and copying back
only `/etc/safeterm-managed` also misses custom external resources and can change
their metadata. Operators need reusable definitions across a fleet without
cloning authentication databases or signing identities.

## Decision

Default upgrades stage the existing verified catalog and retain its executable
hashes and trust. Both local and offline-signed deployments retain customization.
Explicit signed input or portable source is an intentional catalog replacement.
Portable source refreshes target executable hashes and requires local signing;
ordinary updates never implicitly approve changed executable content. A missing
key cannot cause an existing deployment to generate a different trust identity.

Validate catalog/schema, signatures, executable digests, user/resource references,
profile compatibility and the deployment bundle before creating the installation
transaction or stopping services. Unsupported profile migrations fail closed.
Store release defaults separately at `/usr/share/safeterm/defaults`, covered by
package rollback; do not automatically add new operations or privileges.

Install only catalog-owned JSON and reference documentation. Preserve unrelated
site files and initialize authentication/registry files only when absent. Stage
existing registered application files for bundle verification, including external
paths; never overwrite existing managed file contents or ownership with seeds.

Add catalog export containing definitions only. Apply exported definitions with
explicit `--refresh-hashes` using the target's existing key and transactional
activation. Host users, host keys, resource registries, signing material and
filesystem permissions are outside this portable catalog contract.

## Consequences

Site policy survives updates and new release permissions require explicit adoption.
Normalized JSON preserves semantic definitions, not original fragment filenames.
Changed OS binaries require deliberate hash approval. Incompatible sandbox schema
changes need explicit migration. Export is not a full host backup; existing package
and catalog rollback remain separate recovery mechanisms.
