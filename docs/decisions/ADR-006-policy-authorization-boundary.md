# ADR-006: Additive Authorization and the Authentication Boundary in Policy Engine

## Status
Accepted

This ADR supersedes the interpretation of identity verification in the
Consequences section of ADR-002. All other ADR-002 decisions remain unchanged.

## Context
M3 loads roles and permissions from configuration, and M2 establishes where
Policy runs, but neither authenticates the user. Earlier ADR-002 wording could
be read to mean that `ClaimedRoles` becomes trustworthy when M5 arrives. That
interpretation is incorrect: checking authorization is not proving identity.

Behavior for multiple roles and multiple policies for one role must also be
explicit and testable.

## Decision
- `internal/policy.Enforcer` implements the existing `agent.PolicyEnforcer`
  interface; Core and its stage order remain unchanged.
- The constructor receives a valid Config produced by `config.LoadConfig` or
  checked by `config.ValidateConfig`, then builds an independent role-to-
  allowed-operations map. Policy does not repeat referential integrity checks.
- Access is granted only through an explicit permission. A missing role,
  missing policy, or operation absent from the role's permissions causes denial.
- Roles and repeated policies for a role use additive permissions: one grant is
  sufficient. An unknown role grants nothing but does not revoke another valid
  role's explicit permission. Role order has no effect, and there are no
  implicit wildcards or special roles.
- A nil request returns an error. A nil Config and zero-value Enforcer grant no access.
- Policy answers only this question: "If the claimed roles are true, may they
  run this operation?" M5 does not authenticate users or trust roles supplied
  by a Broker.
- M5 integration tests use the real Validator and Policy. Translation and
  Execution remain test doubles and execute no operating-system command.

## Alternatives Considered
1. **Grant access by default when no policy exists** — Rejected because it fails open.
2. **Require every role to grant permission, or reject the entire request for
   one unknown role** — Rejected because it conflicts with additive roles. A
   role without permission should not revoke another role's explicit grant.
3. **Implement authentication inside Policy** — Rejected because this stage's
   contract covers authorization only, and no authentication mechanism has yet
   been specified or designed.

## Consequences
- Tests must prove both access denial and actual termination of stages after Policy.
- Passing Policy does not prove identity. Wiring a real Broker later requires a
  separately designed authentication mechanism and must not trust a user's role claim.
- Placeholders remain available for isolated stage tests; the M5 integration
  path does not use the Policy placeholder.
- Loading and validating configuration is a prerequisite for constructing
  Policy in a real runtime path.
