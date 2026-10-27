# ADR-043: Typed delimited configuration values

Status: Accepted

## Context

Application INI values such as `PATIP` contain comma-separated IPv4 addresses.
A scalar `ipv4` validator correctly rejects them; a plain `string` loses address
validation. Regex type macros would introduce a second schema language and
duplicate the existing scalar validators.

## Decision

Add field type `list` with explicit `item_type` and `separator: ","` to resource
configuration schemas. Validate each nonempty item with the existing scalar
validator. Supported items are string, integer, port, boolean, ipv4, ipv6, ip,
cidr and ip_port. Do not support recursive lists, enum or filesystem path items.

Keep the application value a string in INI and JSON; preserve item order,
spelling and duplicates. Reject surrounding item whitespace and empty items.
Optional positive `min_items` and `max_items` constrain the item count.
`allow_empty` permits a completely empty value, independently of key presence
and item bounds. Pattern and length constraints apply to the whole value.

Expose all list constraints through field descriptions so immediate CLI draft
validation agrees with Manager preview and writer validation. Report the
one-based failing item position without echoing its value; existing secret
redaction and resource/file/key context remain effective.

## Consequences

Existing scalar schemas remain unchanged. New schema options are rejected on
scalar fields. Administrator schema changes require check/apply and reconnect;
old binaries do not understand the new field type. No regex macros, new service,
external dependency or permission change is needed. Existing preservation and
transactional writer boundaries continue to apply.
