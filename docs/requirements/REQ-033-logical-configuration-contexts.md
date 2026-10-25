# REQ-033: Configurable CLI contexts and named fields

1. Register each physical file once and define one or more config.contexts paths.
   Fields have case-sensitive CLI name aliases and exact section/key destinations.
   Reuse existing types, constraints, descriptions, read-only and secret flags.
2. Derive config path help, completion and visible names from authorized metadata;
   preserve existing Tab behavior. Do not expose a raw resource-ID route for
   context-based definitions or accept arbitrary coordinates through set.
3. Offer show/diff/set/end/abort/cancel in singleton contexts. End saves one file
   atomically; external version changes fail without losing the draft. Preserve
   unrelated bytes/fields, ownership, ACLs and existing writer protections.
4. Never automatically activate services on save. Reject source on_change with
   a useful explanation. Service management uses execute service separately.
5. Reject malformed, conflicting or excessive mappings before activation, retain
   resource-level authorization, and copy exposed mapping metadata.
6. Whole-file rollback must explicitly identify its scope and refresh alias values.
7. Remove root show while keeping config-local show. Provide capture instead of
   trace and remove packaged nslookup/traceroute and their install dependencies.
8. Ship inactive templates and complete English Markdown/plain-text examples.

Validation: logical_config_test.go, config_editor_ipc_linux_test.go,
configmanager/contexts_test.go, resources/definitions_test.go, and the disposable
systemd/Console PTY smoke, alongside full verification and security suites.
