# REQ-029: Configurable site roles, commands, binaries and Runner boundaries

- A normal installation works from verified packaged defaults without site files.
- Independent strict JSON directories configure roles, operations, executables,
  policies, profiles and service/config/log resources.
- Each directory ships an always-ignored template and readable English guides.
- Same-ID overrides are explicit; removed overrides restore defaults; exact
  policy lists have deterministic precedence and no wildcard/inherited grants.
- termctl supports default extraction, profile/executable discovery and hash
  inspection, with completion and the existing single check/apply workflow.
- Executable approvals retain explicit paths, SHA-256 and profile membership;
  hashes never auto-refresh during ordinary activation.
- Custom Runner profiles get separately provisioned locked identities/sockets/
  units and bounded template-based privileges; arbitrary root/unit text is rejected.
- Audit/IPC/sudo enforce distinct Runner scopes. Rollback/recovery and package
  update/uninstall handle the generated deployment without modifying user sources.
- Real systemd/Console tests cover custom profiles, execution, rollback and update.

Architecture: [ADR-033](../decisions/ADR-033-declarative-site-catalog-and-provisioned-runners.md).
