# Structured Configuration Management

M13 exposes registered configuration as typed resources and fields. A remote
caller never supplies a path, parser, replacement document, executable, or
shell text. The SSH Broker authenticates the caller, passes its server-owned
identity over an authenticated Unix socket, and routes an opaque resource ID
to one isolated Config Manager profile.

## Restricted-terminal workflow

```text
config ?
config RESOURCE
edit SECTION
show
set KEY VALUE
diff
next
edit ANOTHER_SECTION
set KEY VALUE
end
```

`set` stages values; `next` navigates without saving; `end` validates and commits
all changes once. `cancel` discards the draft. Use `edit -` for root-level fields.
Secret fields use `set KEY` with a hidden prompt. Reads/diffs redact secrets and
set command lines are excluded from history. Concurrent file edits cause a
version conflict, retaining the draft without overwriting the external change.

See [the terminal guide](restricted-terminal.md) for contextual help, quoting,
rollback, permissions and examples. Legacy one-shot config forms are removed.
Internal single-use, creator/session-bound proposals and role checks remain;
users do not handle tokens or require another approver. Administrator `edit`
permissions determine proposing and applying. Unknown registry properties,
including the removed require_separate_approval flag, are rejected.

## Registry and adapters

The generated `/var/lib/safeterm/generated/resources.json` registry selects each exact canonical path, profile, format,
access mode, field schema, roles, validator ID, typed activation operations,
and backup policy. Unknown JSON properties are rejected when loading the
registry. One path can belong to only one resource.

The INI adapter accepts blank lines, full-line `;` or `#` comments, named
`[sections]` (including trailing section-header comments), and `key = value` assignments. Keys and sections contain letters,
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

## External files with unchanged ownership

For a file owned by root or another service, opt in with `privileged_write: true`
in its root-owned resource definition. The unprivileged Config Manager delegates
semantic field writes to a separate restricted root writer. Do not chown the
application file or grant the manager general write access. Use
`sudo termctl config check` and `sudo termctl config apply` to validate and activate
the registry and generated writer sandbox. See
[external configuration files](external-configuration-files.md) for a complete
example, namespace restrictions, metadata preservation and recovery.

## Validation and commit

Managed seed files are owned by the corresponding Config Manager user and
private group. Each systemd manager runs with primary group `safeterm-clients`
for IPC and explicitly includes its own private group in `SupplementaryGroups`.
For the original manager-owned resources, this allows an unprivileged manager to preserve the target GID when creating
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

## Help and back navigation

After login, SafeTerm displays the main command menu automatically. Type `?`
and press Enter for commands at the current level. Contextual help keeps your
input: after `execute ?` and Enter, the prompt still contains `execute `, so you
can continue with a command name. The same applies to `config ?`, `edit ?` and
`set FIELD ?`. Help never executes a command, stages a change or saves anything.
Use Ctrl+U to clear the retained line and start another command. Password and
confirmation prompts keep their normal input behavior.

`config` opens a menu. Choose an entry by typing its name or `edit NAME`;
`config network` opens the interface/route menu. Tab completes menu entries and
editor commands. Every `back` displays the parent menu or available items.

| Command | Effect |
|---|---|
| `back` inside an interface, route or file section | Discard that item's unsaved edits and return to its item list; other staged items remain |
| `back` at an editor's item list | Discard the remaining editor draft and return to the parent configuration menu |
| `back` in a logical configuration context | Discard that context's unsaved edits and return to its parent menu |
| `next` inside a network item or file section | Return to the item list, keeping staged edits |
| `end` inside an editor | Validate and save the editor draft |
| `abort` / `cancel` inside an editor | Discard the whole unsaved editor draft and return to the main prompt |

`back` never writes a file, saves a network candidate or applies networking.
Previously saved network candidates are kept. In navigation menus, `end` returns
to the main prompt without saving anything. Only explicit save/apply commands
change configuration.

For example, from `network/interface/ip0`, `back` discards unsaved changes to
`ip0` and shows the interface list. Another `back` leaves the interface editor
and shows `interface` and `route`; another returns to the configuration menu.
