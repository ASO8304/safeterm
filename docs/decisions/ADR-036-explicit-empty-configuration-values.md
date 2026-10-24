# ADR-036: Explicit empty values and contextual configuration errors

Status: Accepted

## Context

Application INI settings such as PATIP may legitimately exist with an empty value.
An optional IPv4 field previously allowed a missing key but rejected an existing
blank as an invalid address. Replacing typed validation with a regex workaround
makes administration harder. Validation errors also lacked the managed file path
and resource identity needed to locate a problem in multi-resource installations.

## Decision

Add allow_empty (default false) to FieldSchema and FieldDescription. A present
exactly empty string is accepted when allow_empty is true, independently of
required. Required always controls missing keys and, unless allow_empty is true,
retains its existing rejection of blank values. Nonempty scalar values remain
subject to all existing checks. An explicit allowed blank bypasses scalar checks
only; cross-field rules still run and can reject it. Whitespace is not empty.
Existing definitions without the option retain their previous semantics.

Use the same flag in registry compilation, interactive draft descriptions,
manager validation and independent privileged writer rendering. INI blank values
and their comments/layout remain intact. JSON allowed blanks are strings even
for numeric/boolean field types; do not silently substitute zero, false or null.
The editor still cannot create a missing registered key.

Field errors include resource ID, managed file, section/key, type, required and
allow_empty flags, plus numeric bounds. Distinguish missing keys, empty values
and failed type/constraint validation. Never echo secret values; redact underlying
filesystem errors for secret fields. File-read and parse failures in the writer
plan also identify the resource and managed path.

## Validation

Test the required/allow_empty matrix, every supported scalar type, invalid
nonempty rejection, draft checks, INI BOM/comments/CRLF, JSON blanks, manager
describe/preview/apply, contextual errors and secret redaction. Run the full
verification, deployment, adversarial and race suites.
