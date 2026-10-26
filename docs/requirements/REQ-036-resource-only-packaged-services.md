# REQ-036: No implicit restricted-terminal service targets

- Fresh installations expose no packaged service status/control operations.
- `resources.d` definitions supply the visible service aliases and per-action grants.
- Keep service roles, isolation templates, executable bindings and SafeTerm runtime units.
- Generate no service sudo privileges when no service operation is registered.
- Preserve administrator definitions on update and validate obsolete explicit policy references.
- Cover absence of defaults and continued resource service/group operation in tests.

Decision: [ADR-041](../decisions/ADR-041-resource-only-packaged-service-registration.md).
