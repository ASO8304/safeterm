# ADR-004: Use JSON Instead of YAML for Configuration Files (M3)

## Status
Accepted

## Context
The project specification explicitly lists YAML and JSON as acceptable formats
for Declarative Command Config. YAML is often more readable for hand-written
configuration because it supports comments and is less verbose, but the Go
standard library does not support YAML and therefore requires an external
dependency such as `gopkg.in/yaml.v3`.

When that dependency was evaluated in the current development/test environment,
access to `proxy.golang.org` was blocked with `403 Forbidden: Host not in
allowlist`. Consequently, `go get` and `go mod tidy` could not retrieve it in
this environment.

## Decision
M3 uses **JSON** through the standard `encoding/json` package rather than YAML.

## Alternatives Considered
1. **Use YAML with `gopkg.in/yaml.v3`** — Rejected for M3 for two reasons:
   - Operationally, the dependency could not be downloaded and verified in the
     current environment before delivery. The project requires `make verify`
     to pass before any code is delivered.
   - Architecturally, ADR-002 establishes deliberate avoidance of unnecessary
     external dependencies. The standard library's JSON support fully meets
     the current need.
2. **Implement a small internal YAML parser** — Rejected as unnecessary
   overengineering that would add parser defects and maintenance risk.

## Consequences
- Files under `configs/operations/*.json`, `configs/roles/*.json`, and
  `configs/policies/*.json` must be valid JSON, without comments and with
  double-quoted keys.
- This decision can be revisited. If the team later works in an environment
  with full access to `proxy.golang.org` and YAML readability is more valuable,
  a new ADR (for example, ADR-010) can add a YAML-to-struct translation layer
  without changing `OperationDefinition`, `RoleDefinition`, `PolicyDefinition`,
  or `ValidateConfig`.
- `LoadConfig` separates file parsing (`loadJSONArrayFiles`) from validation
  (`ValidateConfig`), reducing the cost of a future format change.
