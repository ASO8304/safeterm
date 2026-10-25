# ADR-037: Logical configuration contexts and field aliases

Status: Accepted. Extends ADR-030 and ADR-031. Automatic service activation in
resource definitions is superseded by explicit execute service actions.

## Context

The restricted terminal should expose logical, administrator-defined commands
such as config tunnel / set Value_1 1400 while editing an existing INI key such
as [Tunnel].TCPMSS. A shared file may serve several services and logical contexts.
Physical file duplication and hardcoded Go command branches would undermine
configuration extensibility. Users selected existing Tab behavior, retained
show/diff inside config, removed root show, and chose explicit service actions.

## Decision

A resources.d configuration accepts config.contexts, each with a token-array path,
optional description and fields. Each field includes name plus the existing
section/key/type/constraints schema. Names are CLI aliases, not value maps.
Contexts and legacy config.fields are mutually exclusive in one definition.
Compilation flattens physical fields into one registry resource per file and
adds validated context/alias references. Physical coordinates are unique in the
resource. All mapping metadata is part of the generated, signed registry artifact.

Registry validation rejects invalid tokens/names, unknown coordinates, duplicate
aliases, duplicate/prefix-conflicting routes (including legacy resource-ID routes)
and excessive shapes. Limits are 8 path tokens, 128 contexts/resource, 512 fields/
context and 4096 total routes, in addition to existing source/registry size bounds.
A trie bounds route validation to route length. Metadata returned to callers is
copied so in-process clients cannot mutate manager routes.

Managers expose contexts in authorized resource summaries. The broker builds
path completion and help from authorized summaries and fails closed on collisions
between isolated manager clients. Legacy resources expose their ID; context-based
resources expose only their declared logical paths, without a raw-ID escape hatch.
Each singleton context directly offers set NAME VALUE, show, diff, end,
abort/cancel, rollback and exit. Nested paths are token sequences, not table entry
creation. Singleton contexts do not expose edit/next. Existing legacy section
editing retains edit/next and its existing end-save behavior. Tab is unchanged;
contextual ? remains the existing question-mark-plus-Enter interface.

Editor schemas/read values use aliases; final patches resolve to registered
physical coordinates. Typed scalar feedback, secret prompts/redaction, history
suppression, bounded drafts and cancellation remain. Manager/writer independently
authorize resources and validate real coordinates. Contexts inherit the resource's
roles; aliases are presentation, not independent field-level RBAC. No service or
file path can be supplied as an arbitrary runtime parameter.

One file retains one version/lock authority. End validates the final physical
document and writes once, preserving unrelated fields, INI layout and metadata.
Concurrent external changes reject stale saves and retain drafts. No multi-file
atomicity is implied. Rollback restores a whole-file backup, not one view; the
confirmation names that scope and refreshes aliased values afterward.

Resource source config.on_change is explicitly rejected with a migration message.
Generated resources have no activation operation, even when service actions are
also defined. Logical registry contexts reject activation/health-check bindings.
Configuration saves only write files; service operations remain separate typed,
authorized execute service commands. Generic internal manager activation support
is not reachable from newly compiled resource definitions.

Capture replaces the packaged trace operation; nslookup/traceroute operations,
executable bindings and installation dependencies are removed. Custom source
policies/overrides referring to removed defaults must be revised explicitly; no
silent source rewriting or legacy trace alias is added.

## Validation

Tests cover source compilation, schema/route/reference conflicts, metadata copies,
alias help/completion, physical-name rejection in logical contexts, read-only
principals, secret redaction, shared-file updates, cancellation, stale saves,
byte-preserving comments/BOM/CRLF and one backup per save. Authenticated IPC tests
exercise SSH and Console with logical paths. The disposable systemd/PTY smoke
adds a real root-owned multi-context file, checks alias completion and exact bytes,
and verifies saving did not restart the registered service. Run verify/e2e/security
and race checks; real root systemd validation remains a CI gate.
