# ADR-032: Single source configuration without legacy fallback

Status: Accepted. Supersedes configuration-source and compatibility provisions
of ADR-031 and earlier catalog/registry administration decisions.

## Decision

Only root-owned `/etc/safeterm/resources.d/*.json` is administrator input.
`TEMPLATE.json` is ignored before opening. Independent service/config/log blocks
compile with verified package built-ins into root-owned
`/var/lib/safeterm/generated`. No previous generated registry, operation, or
inventory participates in compilation. Shipped built-ins live in
`/usr/share/safeterm/defaults`; repository defaults are internal/defaults/catalog.
Credentials and trust remain under `/etc/safeterm`.

`termctl check` validates. `termctl apply` activates the complete result through
one journaled transaction, including registry, writer sandbox, exact systemctl
grants and active services. `apply --rollback` restores the prior activation;
`apply --recover` handles interruption. Package rollback remains separate.
Old catalog/config mutation CLI routes and registry approval flags are rejected.
The restricted terminal accepts only its contextual grammar.

Runtime units use fixed generated-data paths. Audit owns only its own
`/var/lib/safeterm/audit` directory, never generated metadata or its parent.
The restricted writer retains bounded path/metadata validation and no brokers
or managers receive general root/sudo access.

## Breaking deployment boundary

Old layout markers cause installation and activation to fail before deployment;
there is no automatic import, deletion or fallback. Operators must back up and
recreate intended resources on a clean deployment. New-architecture updates
preserve JSON inputs and identities and regenerate runtime data. Local signing
uses an existing protected private key. Offline signatures must match the
compiled candidate; a mismatch or missing signer fails, never weakens trust.

## Consequences and validation

A single readable source replaces competing catalogs/registries and removes
ambiguous precedence. A derived file is never treated as user configuration.
An old-layout upgrade is intentionally unsupported. Tests cover refusal without
mutation, retired commands, source compilation/removal, empty registries,
transaction failure/recovery and deployment paths. Full verify/e2e/security and
both architecture package manifests validate the implementation. Root-only
filesystem integration additionally runs in CI.
