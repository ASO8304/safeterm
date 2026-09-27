# REQ-006: Translate Authorized Operations into a Fixed Command and Separate Arguments

**Source:** M6 — Translation Layer, the continuation prompt's trace example,
and the `agent.Translator` contract
**Priority:** Critical
**Status:** Done

## Description
The `internal/translation` package converts a request that passed Validation
and Policy into a fixed command name from config and a `[]string` argument list.
It replaces `IdentityTranslator` in the M6 integration path and executes no
shell or process.

Every `ArgsTemplate` token is either a literal or a complete `{name}`
placeholder. ADR-007 documents the limited grammar, construction-time checks,
and optional-parameter behavior. Type, range, and pattern checks remain the
Validator's responsibility; authorization remains Policy's responsibility.
Calling Translate directly does not replace those stages.

## Acceptance Criteria
- [x] **AC-1:** trace with port=999 and host=192.168.10.20 translates to command
  tcpdump and exactly `["-i", "any", "port", "999", "and", "host", "192.168.10.20"]`
- [x] **AC-2:** The command is copied unchanged only from config. Literal and
  substitution order is preserved; whitespace or an empty value remains one
  argument. Repeated placeholders are allowed, and no shell interpretation,
  join, or split occurs
- [x] **AC-3:** A nil or empty template produces an empty argument list; the
  shipped system_info maps to uname with `-a` according to repository config
- [x] **AC-4:** The constructor rejects nil Config, invalid Config, and duplicate
  parameter names with an error and no usable Translator
- [x] **AC-5:** Undefined placeholders, incomplete/nested/empty braces, and a
  placeholder combined with a literal are rejected at construction with a clear error
- [x] **AC-6:** A missing value for any parameter referenced by the template,
  including an optional parameter, is an error. Error output always has an
  empty command and nil args rather than partial output
- [x] **AC-7:** A nil request, unknown operation, raw executable name, and
  zero-value Translator are rejected without panic; there is no fallback to `req.Command`
- [x] **AC-8:** The constructor does not mutate Config and creates an independent
  command/template snapshot. Translate does not mutate the request, and each
  call returns independent output
- [x] **AC-9:** Only template tokens appear in output; a defined but unused
  parameter is not automatically appended to arguments
- [x] **AC-10:** In Core with real Config, Validator, Policy, and Translator, a
  test Executor receives the exact final command and arguments for trace and system_info
- [x] **AC-11:** Invalid input and unauthorized roles stop before Translation.
  Translation errors return `rejected` at the translation stage without calling Executor

## Related Tests
- TEST-TRANS-001 through TEST-TRANS-009 (`internal/translation/translator_test.go`)
- TEST-INTEG-007 through TEST-INTEG-009 (`tests/integration/agent_translation_test.go`)

## Related ADRs
- ADR-003 (independent stages and termination after failure)
- ADR-005 (input validation)
- ADR-007 (whole-token templates and shell-free translation)
