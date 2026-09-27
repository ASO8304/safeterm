# ADR-005: Validation Engine Security Decisions (Allowlisting and Defense in Depth)

## Status
Accepted

## Context
The Validation Engine (M4) is the first component that examines actual user
input rather than only configuration structure. Because the project is
explicitly designed to prevent command injection (project specification,
sections 4 and 8), two design decisions require explicit documentation.

## Decision

### 1. Argument Allowlist, Not Only Value Validation
Every argument present in `ExecutionRequest.Arguments` but absent from
`OperationDefinition.Parameters` is rejected regardless of its value.

**Why:** If the validator checked only defined values and ignored additional
arguments, a defective Broker or a future Translation Layer could accidentally
pass an unintended argument to the real system command. Explicitly rejecting
unknown arguments removes this risk at the boundary.

### 2. Defense in Depth: Dangerous-Character Denylist and Mandatory Full Match
Regardless of whether a string parameter has a `Pattern`:

- Every string value is rejected if it contains characters commonly used for
  command/argument injection (`;`, `|`, `&`, `` ` ``, `$`, `(`, `)`, `{`, `}`,
  `<`, `>`, newline, or quotes).
- `Pattern` must match the **entire** value, not only a substring, even when the
  config author did not anchor the expression with `^...$`.

**Why:** Assuming every configured `Pattern` is correctly and fully anchored
is fragile. One human mistake, such as omitting `^` or `$`, could eliminate the
Validation Engine's security benefit. An independent denylist that does not
depend on pattern quality adds a second layer of protection.

## Alternatives Considered
1. **Trust the configured `Pattern` completely without an independent denylist** —
   Rejected because it ties system security to the regex quality supplied by a
   config author who may not be a security specialist.
2. **Globally reject every non-alphanumeric character with a strict allowlist** —
   Rejected as too restrictive. It would prevent future string parameters such
   as file paths containing `/`. The current explicit denylist plus a
   parameter-specific Pattern preserves more flexibility.
3. **Ignore additional arguments instead of rejecting them** — Rejected because
   silently discarding unexpected data makes the failure invisible. Explicit
   rejection makes the event observable and auditable.

## Consequences
- Authors of future configuration (M6 and later) must understand that string
  values remain protected by the denylist even when a `Pattern` is broader
  than necessary. This behavior is intentional.
- If a future operation genuinely requires a denylisted character, such as a
  parameter that accepts `$`, this decision must be revisited through a new ADR
  with explicit security justification rather than silently bypassed.
