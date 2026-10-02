# REQ-013: Configuration Changes Must Be Structured, Validated, and Atomic

**Source:** M13 — Structured Configuration Management and ADR-014
**Priority:** Critical
**Status:** Implemented in M13

## Description
SafeTerm must let an authenticated user interactively inspect and change
registered configuration fields without providing a general file editor or
filesystem access. Broker presents resources, sections, keys, validation
feedback, and diffs. A capability-specific Config Manager performs all reads,
parsing, validation, staging, application, activation, and rollback under a
narrow operating-system identity.

For an INI resource such as:

```ini
[cats]
Cat_Path = "/etc/cat"
Cat_Port = "99"
```

a caller may select the registered `cats` section and `Cat_Port` key and submit
a candidate value. The request contains a resource ID and field coordinates,
not a path or replacement file. The service parses the value according to its
schema, validates the complete staged document, produces a diff, and changes
the active file only after authorized apply.

## Acceptance Criteria
- [x] **AC-1:** One Config Manager implementation can run as multiple isolated
  profiles. Each instance has a separate service identity, socket, verified
  resource subset, filesystem access, and resource limits.
- [x] **AC-2:** A root-owned or signed registry maps an opaque resource ID to an
  exact canonical file, format adapter, profile, access mode, field schema,
  secret metadata, validation operation, activation operation, and backup
  policy. Callers cannot provide paths, parser names, commands, or hooks.
- [x] **AC-3:** The semantic API supports listing authorized resources,
  describing sections and fields, reading permitted values, validating a patch,
  previewing a diff, applying an approved proposal, and rolling back according
  to policy.
- [x] **AC-4:** A patch contains only `resource_id`, `section`, `key`, candidate
  value, and `base_version`. Config Manager reads the current document and
  rejects an unregistered, read-only, secret-only, or unauthorized field.
- [x] **AC-5:** Field validation supports required state, string, integer,
  boolean, enum, range, length, and anchored pattern constraints. Numeric values
  are parsed before validation even if INI quoting represents them as strings.
- [x] **AC-6:** Path fields support allowed canonical prefixes, existence and
  file-type checks, expected owner/mode, and an explicit symlink policy.
  Traversal, canonical-path changes, and access outside the profile fail closed.
- [x] **AC-7:** JSON and a documented INI grammar are supported through format
  adapters with a common parse, describe, validate-patch, and render contract.
  INI preservation of comments, ordering, quoting, and line endings is tested;
  JSON rendering behavior is deterministic and documented.
- [x] **AC-8:** Validation runs in three layers: field/schema validation,
  complete rendered-document validation including cross-field rules, and an
  optional typed application validator. No layer executes a shell string or
  caller-selected program.
- [x] **AC-9:** Preview returns a short-lived proposal token bound to the actor,
  resource, exact normalized patch, base version, rendered diff, and expiry.
  Apply accepts the token and rechecks authorization and file version rather
  than accepting altered patch content.
- [x] **AC-10:** Apply stages the replacement in the managed directory,
  preserves required ownership and mode, validates, fsyncs, creates a bounded
  backup, and uses atomic rename. A pre-commit failure leaves the active file
  unchanged.
- [x] **AC-11:** Optimistic concurrency rejects an apply when the active content
  no longer matches `base_version`. It returns a conflict without overwriting
  the other change.
- [x] **AC-12:** Activation uses a typed, approved service-management operation.
  A configured activation or health-check failure invokes rollback and records
  both results without granting Config Manager general service-control rights.
- [x] **AC-13:** Read, propose, apply, activate, and rollback are independent
  permissions derived from authenticated M12 SSH identity and approval context.
  Unverified claimed roles cannot authorize remote mutation.
- [x] **AC-14:** Secret values are redacted from reads, diffs, errors, and audit.
  Audit records actor, resource ID, field name, version/hash transition,
  proposal/approval identity, and validation/apply/activation/rollback outcomes
  without storing raw sensitive values.
- [x] **AC-15:** Integration and security tests cover valid edits, type/range and
  path rejection, unknown fields, stale versions, proposal tampering and expiry,
  parser failures, symlink/path replacement, partial-write recovery, failed
  activation rollback, cross-profile access, and unauthorized operations.

## Related Tests
- TEST-CONFIG-MANAGER-001..012 cover adapters, schemas, paths, proposals, atomic apply, rollback, IPC, activation failure, Broker routing, and deployment isolation.

## Related ADRs
- ADR-006 and ADR-015 (authentication remains outside Policy; SSH identity is available from M12)
- ADR-009 (capability-isolated services)
- ADR-010 (authenticated local IPC and typed backends)
- ADR-011 (synchronous sanitized audit)
- ADR-014 (structured configuration management service)
