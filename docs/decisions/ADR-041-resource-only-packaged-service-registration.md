# ADR-041: Resource-only packaged service registration

Status: Accepted

## Context

Fresh installations previously exposed SafeTerm's own audit, gateway and SSH
services as predefined restricted-terminal operations. Administrators expect to
choose service targets and aliases through `resources.d`, as they do for logs.

## Decision

Remove every packaged systemd operation and its policy grants. Keep the
`service-viewer` and `service-operator` roles, service-manager isolation profile,
executable bindings and actual SafeTerm systemd units. An empty default service
catalog generates no service sudo grants and no `execute service` menu branch.

Administrator resources generate the authorized per-action operations, exact
sudo grants, aliases and ordered groups. Administrators manage SafeTerm's runtime
services from their Linux shell through `sudo termctl service`. Explicit custom
operation/policy definitions remain administrator-owned and are not deleted on
update. Custom policies referring to removed default IDs require explicit edits;
there is no silent import of the previous generated catalog.

## Validation

Regression tests reject packaged service operations or role grants, ensure an
empty catalog grants no service sudo privileges, and verify resource-generated
service commands still compile and execute. The fresh systemd/Console test checks
that service commands are absent before resources are registered, while the
existing service-group integration test exercises explicitly registered targets.
