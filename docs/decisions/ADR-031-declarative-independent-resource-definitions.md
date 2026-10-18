# ADR-031: Declarative independent resource definitions

## Status

Accepted. Extends ADR-026 through ADR-030. Requires a package containing this ADR;
older published releases do not implement resources.d or termctl check/apply.

## Context

Registering one application requires changes across a field registry, operation
catalog and policies. Configuration files and logs may not belong to a systemd
service. The administrator needs one portable JSON definition per logical
resource, with a permanently installed reference that cannot become active.
Existing customized catalogs, identities and external file metadata must survive.

## Decision

Read root-owned /etc/safeterm/resources.d/*.json only from the administrator's
termctl compilation path. Skip exactly TEMPLATE.json before any read/decode;
ship and refresh this reference and English Markdown/plain-text guides. Preserve
custom files on package updates. Standard JSON only: strict typed decoding,
unknown/duplicate-property rejection, bounded depth/size/count and no comments.
Require id, description and at least one independent service/config/logs section.

Compile exact service actions and fixed-path streaming log operations into the
existing signed catalog, with explicit per-action/log roles. Compile typed fields
into the existing registry using the restricted privileged writer. No new daemon,
Linux account, wildcard operation or general sudo grant. Default profile is
application, backup retention 5, and activation is opt-in. Optional on_change
requires an explicit restart/reload action and its authorization. Verify existing
units, actual files/schema/metadata, and the log reader's unprivileged read access;
never chown or silently grant ACLs to repair failed validation.

Generated operation IDs use a reserved resource_<id>_<kind>_<name> namespace;
the shared terminal maps these to execute service ACTION ID and execute logs ID
LOG. The gateway still receives the signed logical operation and authenticates
roles independently. Reject ambiguous friendly paths before activation.

Preserve legacy definitions. A root-owned generation manifest records only
compiler-owned operation/resource IDs. Replace/remove those on subsequent apply,
without deleting application data, stopping external services or overriding a
legacy ID/path. The generated registry/catalog is output, not another editable
source. Legacy administration remains for explicit advanced/migration use.

Add termctl check and termctl apply. Check needs no private key. Apply uses the
existing local signer, or compares an offline-signed catalog to the exact compiled
candidate. Support unsigned export for offline signing. No implicit key generation,
trust rotation or executable-hash refresh. Before activation, validate all references,
identities, profiles, executable hashes, file paths, writer sandbox and sudoers.

Extend the existing catalog recovery journal/checkpoints with the registry,
writer drop-in, generation manifest and verified registry checkpoint. Stop only
previously active consumers, publish together, reload systemd, run preflight and
restart/check the previous active set. On failure restore all associated metadata
and service state. Retain an unrecovered pending journal; reuse the lifecycle lock
and installer/update/uninstall guards. Unchanged compiled content is a no-op.
A completed catalog rollback includes resource metadata; pending recovery is also
available as termctl apply --recover. Neither rewrites resources.d source files.

## Consequences

Independent config-only/log-only/service-only resources are first-class. A single
JSON file can combine related functionality; a single apply activates it. Users
retain the context editor and permission-filtered command tree. Source portability
requires provisioning referenced units/files, existing roles, OS read access and
target signing; it excludes identities, private keys and generated manifests.

Application configuration retains the existing strict root-controlled namespace
and metadata boundaries. Logs are read as the existing log-reader user, without
new capabilities; initial link/path/read checks do not repair log rotation or
guarantee future application-managed file permissions. Operators must maintain
narrow read/traverse permissions and trusted log paths during rotation. Existing
three Config Manager profiles and legacy seed resources remain installed.

## Verification

Tests cover independent/combined sections, role isolation, strict JSON, protected
paths, generated ownership/removal, collisions, template skipping, offline signing,
no-op apply, coupled failure recovery and retained journals, template refresh and
custom-definition preservation. Root-owned input/link/FIFO/mode boundaries run in
disposable CI as root. Existing writer root/non-root integration, SSH/Console,
full verification, deployment E2E, security and race suites remain required.
