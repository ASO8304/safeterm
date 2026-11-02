# ADR-052: Interface name candidates and numbered link rules

Status: Accepted. Supersedes the naming command and filename choices in ADR-051.

## Decision

Expose persistent naming as `set name NAME` in the interface editor. Include it
in dirty prompts, field diffs, local discard/navigation and saved network
candidates. Remove renaming from execute help/completion; the old command
returns instructions for the editor and performs no mutation.

Save only validates and stores the candidate. Apply trials the MAC-bound Netplan
configuration without a live `.link` override. Confirm commits Netplan followed
by the persistent naming rules. Names take effect on the next reboot/device
activation. Reject conflicting live/persistent names, invalid names, shared MACs,
unsupported sources and names outside current administrator policy.

Reuse each existing single-MAC link file. New rules use the first free filename
from `11-persistent-net.link` through `98-persistent-net.link`, reserving 10 for
administrator rules. Skip files and drop-in directories in all four systemd
configuration search paths. Reserve selected filenames within multi-NIC drafts.
Do not rename existing files solely to change their naming convention.

Extend network recovery transactions with separate optional link originals and
replacement data; runtime overrides contain only Netplan YAML. Restore links and
Netplan on rollback. Finalize fully written naming commits during startup because
udev may already have consumed them during boot. Refuse incomplete restoration
after hardware identities change or when external source edits conflict. Retain
the v0.34.0 standalone naming journal recovery for interrupted upgrades.

## Consequences

Interface settings have one discoverable editor and one save/apply/confirm model.
Renaming now needs confirmation, including name-only candidates. A reboot/device
activation is still a separately planned administrator action. Rollback across
an already activated rename requires Linux administrator recovery.
