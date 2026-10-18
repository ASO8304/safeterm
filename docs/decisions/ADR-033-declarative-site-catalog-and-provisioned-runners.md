# ADR-033: Declarative site catalogs and provisioned Runner profiles

Status: Accepted. Extends ADR-032's administrator-source set without restoring
legacy catalog/registry loading. Supersedes ADR-031/032 provisions that only
packaged roles, operations or profiles can exist.

## Context

Restricting all custom definitions to service/config/log resources prevented
organizations from defining roles, safe native commands and separate Runner
boundaries. Editing packaged or generated catalogs is ambiguous and is not a
supported administration workflow. Defaults should remain usable without edits.

## Decision

Keep signed package defaults and compile complete administrator overlays from
roles.d, profiles.d, executables.d, operations.d, resources.d and policies.d, in
that order. All files are strict root-controlled JSON. TEMPLATE.json is always
ignored. Same IDs replace packaged definitions; deletion of an override restores
the default. Exact policies apply last. Inputs never derive from active/generated
data. Legacy direct catalog directories remain refused.

termctl defaults emits trusted defaults in editable source format; profile and
executable discovery and hash inspection join existing role/catalog commands.
One check/apply/signing/recovery transaction covers the complete derived model,
writer registrations, exact sudoers and generated Runner deployment.

Profiles select one of four fixed packaged isolation templates. Built-in profiles
can change bounded limits and reduce allowed capabilities. Added profiles receive
reserved, deterministic nologin/password-locked system accounts, sockets and
systemd units. They inherit approved template executable membership. No arbitrary
unit text, root user, writable mount, controller or extra capability is exposed.
At most sixteen site profiles are active; memory/process/time/output/session
ceilings are enforced. Reuse of reserved accounts requires a root-private marker,
expected UID class, primary/supplementary groups, disabled password, home and shell.

Audit verifies the signed catalog and binds each site Runner UID to exactly its
own profile. Gateway and Runner IPC continue to authenticate real peer UIDs.
Service-manager sudoers grants are split per Runner and per signed unit/action;
one service profile cannot inherit another profile's root operations.

## Transactions and lifecycle

Definition activation stops consumers before Audit. Signed candidate verification
and host-file validation precede provisioning. Generated units and dependencies
are reconciled from the candidate; recovery/rollback derives them from the signed
checkpoint. Removed profiles lose their unit, Gateway dependency, sudo and Audit
binding. Their locked accounts/markers remain to prevent UID reuse; they cannot
log in. Source files and identity maps are not automatically modified.

Package snapshots include existing site units, Gateway dependencies, account
databases and source/generated state. Update stops old site Runners, preserves
all source directories, installs defaults and re-provisions from current sources.
Rollback and uninstall remove only compiler-marked files in the reserved namespace.

## Validation

Unit tests cover default stability, overrides, roles, exact policies, template
ceilings, strict input parsing, unit generation and CLI default serialization.
Root boundary tests cover source links/ownership. The disposable real-systemd
installation test creates independent process and service-manager profiles, executes
commands through Console, checks UID separation and scoped sudo, removes/restores
profiles with rollback/apply, and updates the package while preserving definitions.

During package staging, existing credential role names may also be checked against
a same-trust signed prior catalog. This is not a compilation source or a runtime
fallback: final installed validation uses only the newly signed compiled catalog.
Unchanged offline updates may reuse a prior signature only after exact candidate
comparison. Changed candidates still require an authorized signer.
