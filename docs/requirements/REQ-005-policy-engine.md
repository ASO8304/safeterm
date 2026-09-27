# REQ-005: Policy Engine Must Authorize Operations from Claimed Roles

**Source:** M5 — Policy Engine, the `agent.PolicyEnforcer` contract, and the M3 role/policy configuration
**Priority:** Critical
**Status:** Done

## Description
The `internal/policy` package replaces `AllowAllPolicy` in the M5 integration
path. It builds a role-to-allowed-operations map from loaded, valid Config and
rejects every request without an explicit grant (fail closed).

`Metadata.ClaimedRoles` remains an unverified claim after M5. Policy checks
authorization for the claimed roles; authenticating users and deriving trusted
roles is the responsibility of a separate future layer (ADR-006).

## Acceptance Criteria
- [x] **AC-1:** A request with nil or empty roles is rejected
- [x] **AC-2:** A permitted role is accepted and an unpermitted role is rejected;
  under shipped config, viewer may run only system_info, while netops may run
  both system_info and trace
- [x] **AC-3:** For multiple roles, one permitted role is sufficient. Role order
  and duplicates do not change the result. An unknown role grants nothing and
  does not revoke another role's permission
- [x] **AC-4:** An unknown or empty role by itself grants no access
- [x] **AC-5:** A defined role without a policy, an empty policy, and a Config
  without policies grant no access
- [x] **AC-6:** Multiple policies for one role merge their grants without
  leaking permissions across roles. The compiled map is independent of later Config changes
- [x] **AC-7:** Given valid Config, an unknown operation or raw system executable name is not authorized
- [x] **AC-8:** Direct calls with a nil request, nil Config, or zero-value Enforcer
  grant no access; a nil request returns an error instead of panicking
- [x] **AC-9:** In Core with real Config, Validator, and Policy, an authorized
  path reaches later stages. Policy rejection returns `rejected` at the policy
  stage before Translation/Execution, while invalid input stops before Policy

## Related Tests
- TEST-POLICY-001 through TEST-POLICY-014 (`internal/policy/enforcer_test.go`)
- TEST-INTEG-004 through TEST-INTEG-006 (`tests/integration/agent_policy_test.go`)

## Related ADRs
- ADR-003 (fixed pipeline order and termination after failure)
- ADR-006 (additive authorization and the authentication boundary)
