// Package policy implements the real PolicyEnforcer (see
// internal/agent.PolicyEnforcer) that decides whether a request's claimed
// roles are permitted to run the requested operation, based on the
// role→operations mapping loaded from configuration (M3, policies.json).
//
// It replaces the M2 AllowAllPolicy placeholder.
//
// ⚠️  IMPORTANT SECURITY BOUNDARY (see REQ-005 and ADR-006) ⚠️
// Enforcer answers exactly one question: "IF this request's claimed roles
// are true, are they allowed to run this operation?". It does NOT verify
// that Metadata.ClaimedRoles are the user's real, authenticated identity.
// That verification is the job of a future authentication layer that does
// not exist yet in any milestone. Until that layer exists and is wired in
// front of every Broker, ClaimedRoles must be treated as attacker-
// controlled input in any threat model of this system — exactly the
// concern REQ-001 flagged when this field was first introduced.
package policy

import (
	"fmt"

	"safeterm/internal/config"
	"safeterm/internal/contract"
)

// Enforcer implements agent.PolicyEnforcer using an in-memory
// role -> set-of-allowed-operations map built from configuration.
type Enforcer struct {
	// allowed[role][operation] == true means that role may run that operation.
	allowed map[string]map[string]bool
}

// New builds an Enforcer from an already-loaded, already-validated Config
// (config.LoadConfig, M3). Because M3's ValidateConfig already guarantees
// every PolicyDefinition references a real role and a real operation,
// Enforcer does not need to re-check referential integrity here.
func New(cfg *config.Config) *Enforcer {
	if cfg == nil {
		return &Enforcer{}
	}
	allowed := make(map[string]map[string]bool, len(cfg.Policies))
	for _, pol := range cfg.Policies {
		ops, ok := allowed[pol.Role]
		if !ok {
			ops = make(map[string]bool, len(pol.Operations))
			allowed[pol.Role] = ops
		}
		for _, op := range pol.Operations {
			ops[op] = true
		}
	}
	return &Enforcer{allowed: allowed}
}

// Authorize implements agent.PolicyEnforcer. It fails closed: a request
// with no claimed roles, or whose claimed roles are all unknown or
// insufficient, is rejected (REQ-005, AC-1, AC-4). If a request carries
// multiple claimed roles, authorization succeeds if ANY one of them grants
// the operation (standard additive RBAC semantics, REQ-005, AC-3).
func (e *Enforcer) Authorize(req *contract.ExecutionRequest) error {
	if req == nil {
		return fmt.Errorf("nil request; access denied by default (fail-closed)")
	}
	roles := req.Metadata.ClaimedRoles
	if len(roles) == 0 {
		return fmt.Errorf("request carries no claimed roles; access denied by default (fail-closed)")
	}

	for _, role := range roles {
		ops, known := e.allowed[role]
		if !known {
			continue // an unconfigured/unknown role grants nothing
		}
		if ops[req.Command] {
			return nil
		}
	}

	return fmt.Errorf("none of the claimed roles %v are authorized to run operation %q", roles, req.Command)
}
