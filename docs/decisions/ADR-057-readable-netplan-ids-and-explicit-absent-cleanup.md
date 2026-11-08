# ADR-057: Readable Netplan IDs and explicit absent-device cleanup

Status: Accepted

Persistent naming previously changed only the MAC-bound `.link` rule. Netplan
kept the original ID, including generated `safeterm-<MAC>` keys. Removed devices
were hidden from active editing, but operators had no cleanup command. These
independent names and retained sources made replacement hardware confusing.

When a supported interface is renamed, change all its Netplan ID fragments to
the requested persistent name and bind the definition to its MAC. Do not add
`set-name`: live names remain unchanged until the `.link` rule is activated.
Editing a legacy renamed interface repairs its ID; an explicit repeated name
edit also requests repair and remains recorded across candidate saves. Scope
this migration to edited or explicitly named interfaces so an unrelated legacy
configuration does not block editing another device. Preserve source locations,
unknown settings, comments and unrelated bytes. Refuse existing target IDs and
references from other definitions instead of guessing at advanced dependencies.

Retain absent definitions by default. Expose them separately with `show absent`;
`delete NAME` stages deletion of an absent definition and its stored routes.
Present devices cannot use this command. Policy, generated-source and renderer
restrictions still apply, as do conservative match/reference checks. An absent
ID is authorized by its stored ID, not a client-supplied or hypothetical live
name. Older candidates without an absence inventory imply retention, never
deletion. Empty inventories in new candidates explicitly preserve staged cleanup.

Both changes use the existing private candidate, source/hardware version checks,
audit, timed trial, atomic confirmation, crash recovery and rollback. They never
write sources at save or install time. A retained target ID must be explicitly
deleted before its name can be reused. Current hardware name/MAC conflicts and
existing `.link` reservations remain independently enforced.
