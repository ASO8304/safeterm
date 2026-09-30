# ADR-014: Structured Configuration Management Service

## Status
Proposed

## Context
Operators need an interactive `config` workflow for a controlled set of service
configuration files. Files can use different formats, including JSON and INI.
The workflow must let a user select an allowed resource, inspect sections and
keys, validate a replacement value, preview the change, and apply it safely.

A generic file editor or interactive shell would expose arbitrary paths and
content to the Agent. One configuration process with write access to every
service would also recreate the universally privileged Agent that ADR-009
removed. The design therefore needs a semantic resource boundary and the same
least-privilege deployment model used by capability-specific Runners.

## Proposed Decision
- Introduce one Config Manager binary that can run as multiple independently
  deployed capability profiles, for example web, database, and network
  configuration managers. Each instance has its own service identity, Unix
  socket, resource subset, filesystem permissions, and systemd sandbox.
- A root-owned resource registry assigns a stable resource ID to an exact file,
  format adapter, profile, read/write mode, schema, secret fields, validation
  hook, activation hook, and backup policy. Callers select a resource ID; they
  never submit a filesystem path, parser name, command, or shell string. The
  registry may live in the signed project catalog, but each instance receives
  only its verified, profile-filtered subset.
- Broker owns the interactive experience. It calls semantic operations such as
  `ListResources`, `DescribeResource`, `ReadSection`, `ValidatePatch`,
  `PreviewDiff`, `ApplyPatch`, and `Rollback`. A terminal menu and a web form can
  use the same API without a PTY.
- Format support is adapter-based. Every adapter parses a document into a typed
  view, describes selectable sections and keys, validates a typed patch, and
  renders a replacement. JSON and a deliberately limited INI grammar are the
  initial adapters; each additional format requires explicit tests and review.
- Validation is layered: field type, required/optional state, enum/range/pattern,
  cross-field schema rules, then an optional typed application validator. Hooks
  refer to approved operations and cannot contain arbitrary executable text.
- A write is transactional. Config Manager checks an optimistic content version,
  stages a file in the same directory, preserves required owner and mode,
  validates the complete rendered document, fsyncs it, creates a bounded backup,
  and atomically renames it. A failure before rename leaves the active file
  unchanged; a failed activation can invoke the declared rollback policy.
- Exact allowlisted paths are canonicalized. Symlink traversal, path replacement,
  non-regular files, unexpected owner/mode, stale content versions, and paths
  outside the instance's resource set fail closed. OS filesystem permissions
  remain the final enforcement boundary.
- Read, propose, apply, activate, and rollback are separate authorization
  actions. Remote write/apply is unavailable until M12 supplies authenticated
  user identity and approval context; unverified `ClaimedRoles` are insufficient.
- Secret fields are redacted in reads and diffs and are never written to audit.
  Audit records actor, resource ID, field names, version/hash changes, approval,
  validation outcome, apply result, activation result, and rollback metadata.
- Service reload or restart is delegated through a typed, narrowly authorized
  service-management operation after the file commit. Config Manager does not
  receive broad service-control privileges.
- This service does not expose raw file editing, arbitrary stdin, an interactive
  shell, a general-purpose editor, or user-selected validation commands.

## Alternatives Considered
1. **Open an allowlisted file in Vim over a PTY** — Rejected because editor
   commands, subprocess escapes, terminal handling, and arbitrary text changes
   undermine typed validation and the narrow semantic contract.
2. **Run one Config Manager with access to all managed files** — Rejected because
   compromise would cross every service boundary and violate least privilege.
3. **Let Broker read and write host files directly** — Rejected because remote
   presentation code must not receive host filesystem authority.
4. **Accept caller-provided paths and infer formats** — Rejected because it
   enables path traversal, parser confusion, and access outside the registry.
5. **Overwrite files in place** — Rejected because partial writes and crashes can
   corrupt the active configuration and make rollback unreliable.
6. **Run arbitrary validator or reload commands from registry strings** —
   Rejected because configuration data would become a command-execution surface.

## Consequences
- The requested interactive workflow fits the microservice architecture without
  adding a shell or expanding Agent Core.
- Multiple service instances can share one implementation while preserving
  separate OS-level blast radii.
- Format adapters must define preservation behavior. INI should preserve comments
  and ordering where practical; JSON may use a documented canonical rendering.
- Atomic rename requires write access to the containing directory. Managed
  files should therefore live in profile-dedicated directories without
  unrelated sensitive files. Legacy shared directories require a separately
  reviewed narrow write helper; granting broad directory write access is not
  an acceptable shortcut. The resource registry is read-only to the service.
- Authenticated identity, approval workflow, and Broker adapters from M12 are
  prerequisites for remote mutation. This capability should be scheduled as a
  dedicated post-M12 milestone rather than folded into M11 streaming.
