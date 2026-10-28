# ADR-044: Group administrator configuration by purpose

Status: Accepted

The flat administrator root mixed credentials, definition directories, network
policy and several long guides. Administrators need one consistent structure.

Keep `/etc/safeterm` as the only SafeTerm administrator root. Group users, roles
and operation permissions under `access/`; operation, executable and Runner
definitions under `commands/`. Keep `resources.d/` and `trust/` at the root.
Place the network policy at `network/policy.json`, and long guides under `docs/`.
The root README explains the tree and the check/apply workflow.

The release installer migrates v0.27 human-owned inputs by renaming them after
the installation snapshot and consumer shutdown. All old/new path collisions
are checked before moving anything; there is no merge or runtime fallback.
Renames preserve file contents, ownership, permissions and ACLs. Interrupted
migration can be repeated. Package rollback restores the original tree and
matching binaries together. This does not import legacy compiled catalogs;
ADR-032's refusal of those layouts remains in force.

The installed runtime reads only the grouped paths. Network policy edits take
effect on the next request; existing terminal editors must be reopened.
Definitions retain the existing `termctl check` and `termctl apply` workflow.
Application configuration remains at its native registered path. Remove only
empty legacy demo directories during successful upgrades, and stop shipping
unused demo seed files in production packages. Preserve and report legacy
directories containing data, symlinks or unknown entries.
