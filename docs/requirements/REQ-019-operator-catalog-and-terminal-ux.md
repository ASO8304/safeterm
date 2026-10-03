# REQ-019 — Operator Catalog, Log Streaming, and Terminal Guidance

**Source:** M19 — Operator Catalog and Terminal UX
**Status:** Implemented in M19

## Requirement

Administrators shall have self-documenting catalog templates for adding safe
operations. Operators shall be able to stream allowlisted logs and invoke exact
typed systemd actions through role-separated access, while the restricted
terminal teaches valid workflows and supports simple positional values.

## Acceptance criteria

- [x] Commented JSONC templates document every catalog object and option while
  remaining outside the runtime `*.json` loader.
- [x] String parameters may define an alias-to-fixed-value map; validation
  accepts only map keys and translation never forwards the caller's alias as a
  filesystem path.
- [x] `run log service_A` invokes fixed `tail -n 50 -F -- PATH` through an
  independent unprivileged log-reader Runner and supports semantic Ctrl+C.
- [x] The installer creates the log-reader identity, unit, readable demo log,
  audit producer binding, and target integrity entry.
- [x] Typed systemd operations support start, stop, restart, reload, enable,
  disable, and status without caller-selected units or actions.
- [x] The installer generates and validates a no-wildcard sudoers command
  allowlist from the signed catalog.
- [x] `service-viewer` receives status-only operations; mutating operations
  require the separately granted `service-operator` role.
- [x] Help provides a quick-start workflow, topic help, operation examples,
  positional and named syntax, editing keys, and mapped-value completion.
- [x] Unit, integration, security, deployment, and shell tests cover the new
  translation and privilege boundaries.
