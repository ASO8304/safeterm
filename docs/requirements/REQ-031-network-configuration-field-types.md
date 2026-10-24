# REQ-031: Network configuration field types

1. Resource configuration fields accept ipv4, ipv6, ip and cidr types with
   strict standard-library parsing and no DNS lookup.
2. Reject wrong families, invalid syntax/prefix lengths, zones, mapped IPv6,
   ports, hostnames, empty values and the literal any.
3. Apply the same validation to drafts, manager operations and privileged writes.
4. Preserve valid spelling and CIDR host bits; keep INI comments and layout.
   JSON values remain strings.
5. Permit additional length/pattern restrictions while rejecting incompatible
   enum, numeric and path options. Do not broaden operation parameter types.
6. Support port as an integer in 1..65535 without explicit min/max. Optional
   limits may narrow the range but cannot widen it. JSON port values are numbers.
7. Ship updated inactive templates and Markdown/plain-text administrator guides.

Tests: internal/configmanager/network_types_test.go and existing integration,
resource compilation, terminal and security suites.
