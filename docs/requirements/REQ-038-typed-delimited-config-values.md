# REQ-038: Typed lists in application configuration

- A resource field can declare `type: "list"`, a supported scalar `item_type`
  and explicit comma separator.
- Each present nonempty value contains at least one independently valid item;
  malformed IPs, empty items and surrounding item whitespace are rejected.
- Optional positive min/max item counts and whole-string constraints are enforced.
- Required key presence and explicit empty-value allowance retain their meanings.
- CLI draft, Manager preview/apply and writer validation use the same constraints.
- INI comments, layout and unrelated values are preserved, as are list order,
  duplicates and spelling. JSON stores this field as a delimited string.
- Errors retain resource/file/section/key context and identify the offending
  item position without exposing secret values.

See ADR-043 and `configs/resources.d/README.md` for schema and CLI examples.
