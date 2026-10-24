# REQ-032: Empty configuration values and actionable check errors

1. An explicit allow_empty flag permits a present empty string independently of
   required; a missing required key must still fail.
2. Without the flag retain existing semantics; nonempty values retain type and
   constraint validation. Cross-field rules remain authoritative.
3. Preserve the flag through compiled metadata and interactive descriptions and
   enforce it in drafts, manager operations and privileged writer validation.
4. Preserve INI blanks, comments/layout and JSON string blanks without inventing
   numeric/boolean defaults. Do not create missing keys.
5. Field failures identify resource, managed path, section/key, type, presence
   and empty flags and relevant numeric limits; secret values are never printed.
6. Document the truth table and an optional PATIP IPv4 example in installed guides.

Tests: internal/configmanager/allow_empty_test.go and existing configuration,
resource compiler, terminal and security suites.
