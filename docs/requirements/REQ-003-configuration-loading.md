# REQ-003: Configuration Loading and Structural Validation

**Source:** Project specification section 6.1 (configurability): define
operations, roles, and policies through configuration files without changing Core
**Priority:** Critical
**Status:** Done

## Description
The runtime loads catalog metadata, allowed operations, executable bindings
with SHA-256 digests, Runner profiles, roles, and role-to-operation policies
from JSON files. M8
extends the original M3 format with `catalog.json`, `executables/`, and
`profiles/` while retaining the same fail-fast loading contract. Loading must:

1. Support multiple files in each directory, so adding an operation means
   adding a file rather than editing one large existing file
2. Fail fast on corrupt, incomplete, or inconsistent files and never return a
   partially valid `Config`
3. Check referential integrity so every policy that references a missing role
   or operation is rejected during loading rather than at execution time

**Format decision:** JSON is used instead of YAML (see ADR-004).

## Acceptance Criteria
- [x] **AC-1:** A complete, consistent config loads and validates successfully
- [x] **AC-2:** Duplicate operation names are rejected
- [x] **AC-3:** An operation without a valid profile, executor kind, and typed
  backend definition is rejected
- [x] **AC-4:** A policy referencing an unknown role is rejected
- [x] **AC-5:** A policy referencing an unknown operation is rejected
- [x] **AC-6:** An int parameter may not have a pattern, and a string parameter
  may not have min/max; min may not exceed max; unknown types are rejected
- [x] **AC-7:** `LoadConfig` reads valid JSON files from disk
- [x] **AC-8:** `LoadConfig` returns an error, not a panic, when a directory is missing
- [x] **AC-9:** `LoadConfig` returns an error for invalid JSON
- [x] **AC-10:** When `ValidateConfig` rejects the final result, `LoadConfig`
  returns `nil, error` rather than an incomplete Config
- [x] **AC-11:** Multiple JSON files in one directory are merged
- [x] The complete shipped catalog under `configs/` loads and validates
  successfully as a permanent regression test

## Related Tests
- TEST-CONFIG-001 through TEST-CONFIG-016
  (`internal/config/validation_test.go`, `internal/config/loader_test.go`)

## Related ADRs
- ADR-004 (JSON instead of YAML for M3)
- ADR-009 (M8 profile and executable catalog extension)
