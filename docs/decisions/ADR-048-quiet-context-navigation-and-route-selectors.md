# ADR-048: Quiet context navigation and route selectors

Status: Accepted. Supersedes the navigation/help behavior in ADR-045 and ADR-046.

## Decision

Make exit contextual: leave one configuration level while discarding that item's
draft; close the session at root only. Remove back. Retain next for staged edits,
save for committing, cancel for whole-editor discard and Ctrl+] for immediate
session exit. Restrict whoami to root. Do not print help on context transitions;
only login and explicit help requests print command menus.

Separate navigation children from editor control commands in empty-line Tab
completion. Configuration menus offer child paths, editor collections offer
items, and selected editors stay quiet until a command or field is requested.
Support relative multiword paths after edit and direct child/item selection.

Assign route display selectors in the cached editor, retaining the actual route
names in all changes submitted to the manager. Reserve deleted selectors until
that editor ends. Avoid numeric stored-name collisions, keep generated hash names
out of normal views and prompts, and expose route detail as labeled fields.
Use add NAME explicitly for creation; edit selects existing routes only. No
selector is a persistent route ID or permission grant.

## Consequences

Admin scripts must replace back with exit and use add for new routes. Network
save/apply/confirmation, ownership, policy, signatures and privilege boundaries
are unchanged. Tests must distinguish context exit from session exit and verify
that menu/help output is on demand, selectors remain stable, and submitted
changes retain their original privileged identities.
