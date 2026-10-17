# ADR-029: Restricted privileged writer for external application configuration

## Status

Accepted. Extends structured configuration management and registry activation.

## Context

An unprivileged Config Manager cannot atomically replace another service's file
while preserving ownership. ACL write access does not permit chown and does not
remove systemd read-only mounts. Making the whole manager privileged would expose
the user-facing proposal, identity and parsing surfaces to unnecessary authority.
Existing INI comments and layout must survive a selected-value change.

## Decision

Add explicit per-resource `privileged_write`. Keep managers and brokers
unprivileged. Introduce a separate root file writer with narrowly bounded
capabilities, authenticated Unix IPC, fixed registry/state/socket paths and no
subprocess execution. Map each peer manager UID to one profile. Accept resource
IDs and semantic field patches with expected versions, never paths or documents.
Revalidate schema, fields, readonly/access flags and complete-document constraints
inside the writer; reconstruct the change independently of the manager's preview.
Keep user roles, authenticated identity and proposal tokens in the existing manager.

Require root-controlled parent namespaces and deny sensitive targets, symlinks,
hard links and ambiguous/unsafe metadata. Generate dedicated writable directory
mounts from the root registry; do not grant managers new filesystem write access.
Preserve UID/GID/mode/access ACLs/xattrs with failure before rename when metadata
cannot be preserved. Audit writer authorization/completion independently. Back up
exact bytes privately as root and bound retention. Reject stale commits/restores.

Add `termctl config check/apply` for root registry validation and sandbox activation,
a verified registry checkpoint and durable recovery journal. Restore prior registry,
drop-in and active-service state on activation failure; retain unrecovered journals
and block package/catalog lifecycle operations until `termctl config rollback`.

Retain INI token editing and add byte-exact large-file tests. Preserve section
comments, inline comments, whitespace, quotes, ordering and line endings. Reject
multiline/ambiguous new values and duplicate sections rather than reformatting.

## Consequences

Root privilege exists in a small dedicated local writer, not the terminal or
Config Manager. Trusted managers can request registered editable fields only in
their own profile; they remain the user-authorization authority. The trusted root
registry is a security policy, not untrusted user input. Service-owned writable
directories require a separate safe integration and are refused here. Atomic
replacement changes inode/mtime, and external application writers must coordinate.
JSON retains deterministic formatting; INI preservation is stronger. Root-owned
and foreign-owned real-file integration is exercised by a non-root child in CI.
