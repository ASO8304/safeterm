# ADR-035: Typed IP and CIDR configuration fields

Status: Accepted

## Context

Administrators need to register IP addresses in managed application configuration
without writing complex regular expressions. These fields must use the same
validation in interactive drafts, manager previews and the privileged writer.

## Decision

Add lowercase ipv4, ipv6, ip and cidr FieldType values. Parse using the standard
Go net/netip package without DNS or network access. IPv4 and IPv6 fields constrain
the family; ip accepts either and cidr requires an address and prefix length.
Reject malformed input, hostnames, surrounding whitespace, zones, brackets,
ports, IPv4-mapped IPv6 and the literal any. Valid unspecified addresses and
zero-length prefixes are accepted; administrators can narrow them with pattern.

Return the original string after validation. Do not canonicalize IPv6 spelling
or mask host bits in CIDR values: application files may contain interface
addresses, and validation must not silently alter their meaning. Network values
are strings in JSON and preserve the INI editor's existing byte-level layout.
They support the same length and pattern constraints as strings, but reject
numeric, enum and path schema options. Empty present values fail even for optional
fields; missing optional fields retain the existing document behavior.

Use the shared normalizeValue path so Console/SSH draft checks, Preview/Apply,
complete-document checks and independent writer validation enforce the same
rules. Resource compilation uses the existing typed registry and signed resource
metadata; no new privileged action, capability or network access is introduced.
Operation request parameter types remain unchanged.

## Validation

Table-driven tests cover families, prefix bounds, malformed input, scoped/mapped
addresses, draft checks, schema constraints and byte-preserving INI edits with
comments/BOM/CRLF plus JSON string round trips. Reuse the configuration manager,
resource compiler, terminal, deployment and security suites.

## Port fields

The port type reuses integer parsing and numeric rendering with unconditional
bounds 1..65535. Zero is excluded because managed service ports must identify an
explicit listening/remote port rather than request ephemeral allocation. Optional
min/max may narrow this range; schemas attempting to widen it are rejected.
Network-address types remain strings, while JSON port values are numbers.
