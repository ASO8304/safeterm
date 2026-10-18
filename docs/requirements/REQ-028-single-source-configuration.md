# REQ-028: One configuration source

- Accept administrator definitions only from resources.d JSON; always ignore TEMPLATE.json.
- Rebuild runtime metadata from verified built-ins and current sources, without legacy merge.
- Separate credentials/trust, generated metadata, packaged defaults and audit ownership.
- Remove catalog/config mutation routes and one-shot restricted editor grammar.
- Reject old installed layouts without mutation, automatic import or deletion.
- Preserve new-format custom definitions and credentials on package update.
- Activate and recover all derived permissions/registry data in one transaction.
- Preserve managed file ownership, ACLs and INI comments; fail closed on unsafe targets.

Architecture: [ADR-032](../decisions/ADR-032-single-source-configuration.md).
