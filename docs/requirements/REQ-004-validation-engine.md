# REQ-004: Validation Engine Must Check Request Arguments Against Parameter Definitions

**Source:** Project specification sections 4 (trace/tcpdump example) and 6.3
(Validation): define rules for each input parameter, enforce the range 1–65535,
and never place input directly into a raw command
**Priority:** Critical
**Status:** Done

## Description
Validation Engine replaces the M2 `PassThroughValidator` placeholder. For each
`ExecutionRequest`, it finds the corresponding operation in the configuration
loaded in M3 and checks supplied arguments against each `ParameterDefinition`.

Two central security decisions are detailed in ADR-005:

1. **Argument allowlist:** Every argument absent from the operation definition is rejected.
2. **Defense in depth:** Regardless of the configured `Pattern`, every string
   is checked against a denylist of dangerous shell characters (`;`, `|`, `&`,
   `` ` ``, `$`, and others), and `Pattern` must match the complete string.

## Acceptance Criteria
- [x] **AC-1:** An operation absent from config is rejected
- [x] **AC-2:** A missing required argument is rejected
- [x] **AC-3:** For the specification's exact `port` example: 80 ✅, 0 ❌,
  65535 ✅, 65536 ❌, and `"abc"` ❌
- [x] **AC-4:** The command-injection attempt `port="; rm -rf /"` is rejected
- [x] **AC-5:** A `host` value that does not match the IPv4 pattern is rejected
- [x] **AC-6:** An argument absent from the operation definition is rejected
- [x] **AC-7:** Dangerous shell characters are always rejected, even when the
  parameter `Pattern` could theoretically allow them
- [x] **AC-8:** `Pattern` must match the complete value rather than only a prefix,
  even if the config author omitted `^...$` anchors

## Related Tests
- TEST-VAL-001 through TEST-VAL-013 (`internal/validation/validator_test.go`)
- TEST-INTEG-001 through TEST-INTEG-003 (`tests/integration/agent_validation_test.go`):
  real M2+M3+M4 wiring for the specification's exact `trace 192.168.10.20:999` example

## Related ADRs
- ADR-005 (Validation Engine security decisions: allowlisting and defense in depth)
