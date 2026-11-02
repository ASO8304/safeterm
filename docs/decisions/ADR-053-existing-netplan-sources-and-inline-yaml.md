# ADR-053: Existing Netplan sources and inline YAML

Status: Accepted. Supersedes the new-file choice in ADR-051.

## Decision

Add new NIC definitions to an existing trusted `/etc/netplan` YAML source instead
of generating `90-safeterm-interfaces.yaml`. Select the unique eligible source,
or the administrator's explicit `new_interface_file` basename in network policy.
Prefer ordinary sources over the legacy SafeTerm file. Refuse missing, ambiguous,
shadowed, policy-denied, generated-without-acknowledgement or unsupported-renderer
destinations. Existing interface entries continue using their original sources.

Accept valid inline Ethernet mappings. Before rendering a changed inline source,
normalize it in memory to block YAML, retaining comments and unknown properties.
Rebind source spans to the normalized document without mutating the baseline.
New interface nodes use block style. Apply insertion patches with the same offset
in parent-before-child order so combined existing/new interface edits cannot put
a child field under the newly inserted interface.

Keep save, temporary trial, confirmation, rollback, ownership, metadata, audit and
stale-source checks unchanged. Retain absent-file recovery for v0.34/v0.35 journals
but do not create new Netplan files in the normal editor path.

## Consequences

Administrators can keep new hardware configuration in their established source.
Multiple source files require an explicit destination choice. Legacy generated
inline configurations remain editable and are formatted on commit; existing
files are not silently migrated or deleted. Rollback restores original bytes,
including inline formatting.
