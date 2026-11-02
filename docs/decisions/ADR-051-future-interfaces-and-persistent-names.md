# ADR-051: Future interfaces and persistent names

Status: Accepted.

## Decision

Add the explicit `allow_all_interfaces` policy opt-in, default false, for current
and future supported Ethernet hardware/VM NICs. Preserve exact allowlists and all
role, renderer, source, generated-file and supported-feature restrictions.

Discover unconfigured present NICs and expose kernel MAC addresses as transient
view metadata. Save/apply/confirm creates MAC-matched definitions in the fixed
`90-safeterm-interfaces.yaml` source when permitted. Extend recovery to distinguish
an absent original file from an existing file; rollback removes only a file
created by that transaction. Reject conflicting vendor files and ambiguous MACs.

Use `execute network rename CURRENT NEW` for persistent naming. It updates one
existing root-trusted, single-MAC `.link` rule in `/etc/systemd/network` and only
creates a MAC-specific rule when none exists. Preserve unrelated bytes and
metadata, clear NamePolicy, reject shared/ambiguous matches and drop-ins, and
bind Netplan to the same MAC while removing conflicting set-name entries.

The operation performs no live rename, udev trigger, Netplan apply, restart or
reboot. It requires editable current/future names, no saved candidate, lifecycle
locking, temporary-root Netplan validation, stale-source checks and audit
authorization. Write a recovery journal before changing files, commit the link
last, complete fully written transactions after a crash and restore partial
ones. Never undo an already completed boot naming change during late startup.

## Consequences

Admins identify a VM NIC by MAC, explicitly opt into future hardware if desired,
and choose a planned reboot/device activation for name changes. Both probes and
naming retain dedicated execution boundaries. The network manager can write
the specific OS networking directories; terminal users cannot supply file paths
or raw link/Netplan content. Older versions do not understand these policy and
created-file features and must not be used to administer them.
