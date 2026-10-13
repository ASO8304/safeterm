# Structured Configuration Management

M13 exposes registered configuration as typed resources and fields. A remote
caller never supplies a path, parser, replacement document, executable, or
shell text. The SSH Broker authenticates the caller, passes its server-owned
identity over an authenticated Unix socket, and routes an opaque resource ID
to one isolated Config Manager profile.

## Restricted-terminal workflow

```text
config list
config describe RESOURCE
config show RESOURCE
config edit RESOURCE SECTION KEY
config apply PROPOSAL_TOKEN
config rollback RESOURCE
```

Use `-` as `SECTION` for a root-level JSON field. `config edit` reads the
current content hash, accepts one candidate value, validates it, and prints a
redacted diff. Non-secret values are normalized before preview. Secret input is
not echoed and secret values never appear in reads, diffs, errors, or audit.
The proposing user confirms the preview with y/N and applies their own change.
No second identity or approval role is required. Tokens are single-use,
creator/resource/patch/version-bound, and kept only in memory. A different
identity cannot consume another user's token. The propose_roles list controls
both proposing and applying; legacy apply_roles is normalized to that list.
Legacy require_separate_approval is accepted but ignored during upgrades.

## Registry and adapters

The root-owned registry selects each exact canonical path, profile, format,
access mode, field schema, roles, validator ID, typed activation operations,
and backup policy. Unknown JSON properties are rejected when loading the
registry. One path can belong to only one resource.

The INI adapter accepts blank lines, full-line `;` or `#` comments, named
`[sections]`, and `key = value` assignments. Keys and sections contain letters,
digits, `.`, `_`, or `-` and begin with a letter. Values can be unquoted,
single-quoted, or JSON-style double-quoted strings. Inline `;` and `#` comments
must follow whitespace outside quotes. Duplicate sections/keys, malformed
quotes, NUL bytes, and ambiguous assignments are rejected. Editing preserves
comments, ordering, whitespace around assignments, quote style, and LF/CRLF
line endings.

The JSON adapter requires an object root and rejects trailing data and
duplicate object keys. A section names a top-level object; an empty section
selects the root. Registered fields must be scalars. Rendering uses two-space
indentation, lexicographically sorted object keys as provided by Go's JSON
encoder, and one trailing newline, so the same document renders identically.

## Validation and commit

Managed seed files are owned by the corresponding Config Manager user and
private group. Each systemd manager runs with primary group `safeterm-clients`
for IPC and explicitly includes its own private group in `SupplementaryGroups`.
This allows an unprivileged manager to preserve the target GID when creating
backups and replacement files. It does not require `CAP_CHOWN`. If an older
installation reports `preserve backup owner: operation not permitted`, verify
the target UID/GID and the service's supplementary groups before retrying.

Validation parses strings, integers, booleans, enums, and paths before applying
range, length, anchored-pattern, required-state, path, and cross-field rules.
Paths are absolute and clean, resolve under an allowed canonical prefix, and
can enforce existence, file type, owner, mode, and a deny-by-default symlink
policy. The staged document is rendered, reparsed, and completely validated;
an optional compiled application validator can then inspect it. Validators and
activation use typed interfaces and cannot be selected by the caller.

Apply rechecks authorization, token expiry, and the SHA-256 base version. It
creates and fsyncs a bounded backup, writes and fsyncs a same-directory
temporary file with preserved mode and ownership, atomically renames it, and
fsyncs the directory. A stale version or pre-commit failure leaves the active
file unchanged. Configured activation and health checks are fixed catalog
operations sent through Node Gateway. A failure restores the backup when the
resource enables automatic rollback.

Gateway authenticates the Config Manager with `SO_PEERCRED` and maps that UID
to exactly one configured profile. It derives the manager's allowed activation
and health operations from the same root-owned resource registry used by the
managers, then intersects them with the signed operation catalog. Only known,
argument-free `systemd` operations are accepted. A manager cannot claim another
profile, forge SSH Broker metadata, request streaming, or submit parameters.

Every read and mutation lifecycle record is sent synchronously to the Audit
Service. Records include the actor, profile, resource and field, version
transition, proposal and approval identities, and outcome without raw values.
