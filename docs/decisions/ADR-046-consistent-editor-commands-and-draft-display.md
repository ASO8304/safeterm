# ADR-046: Consistent editor commands and draft display

Status: Accepted

## Decision

Supersede the editor vocabulary in ADR-026, ADR-033, ADR-040 and ADR-045 with
`save` and `cancel`. Save validates and commits the current application editor;
network save validates and stores a candidate for the separate timed apply flow.
`back` discards the item being left and `next` retains it. Logical contexts may
retain up to 16 context drafts per session, resumed only through authorized
configuration paths. Each retains its original version for conflict detection;
saving one context does not save another. Parent-menu back discards drafts under
that menu; menu cancel and session exit discard retained drafts.

Resolve `exit`, `clear`, and `whoami`, including read-only contextual help, before
dispatching to an editor. Warn on dirty exit. Compute dirty state from effective
configuration values rather than counting network edit operations. Secret fields
remain dirty once set because the terminal cannot compare the original secret.

Present `tcp-test` as the canonical name for the existing signed `telnet` operation.
Resolve the deprecated telnet alias through the same authorization path and print
its migration notice only on execution. Never rewrite signed catalog IDs or grant
permissions for the presentation alias.

Render local diffs as human-readable removed/added lines and keep secret values
redacted. Interface tables separate kernel operational state from permission to
edit. Read `/sys/class/net/*/operstate` in the network manager and expose states
only for discovered interfaces. State is display metadata in View, outside the
source version hash, field comparisons, rendering and transaction target selection.
Refresh it when reading an existing candidate and omit it from persisted candidates
so package rollback keeps the original candidate format. The terminal uses its cached view;
show, help and local diff never invoke a privileged mutation.

## Consequences

Command examples and packaged admin guides use one vocabulary. Old end/abort
commands must be updated. Kernel state in an editor is a snapshot; live status
remains `execute network status`. Missing CIDR prefixes fail immediately, while
full network validation and the privilege boundary remain enforced on save/apply.
