package policy

import (
	"testing"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/contract"
)

var _ agent.PolicyEnforcer = (*Enforcer)(nil)

// testPolicyConfig mirrors the shipped configs/policies/policies.json:
// viewer -> [system_info], netops -> [system_info, trace].
func testPolicyConfig() *config.Config {
	return &config.Config{
		Version: "test-v1",
		Profiles: []config.ProfileDefinition{
			{Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/safeterm/diagnostics.sock", RunAsUser: "test", MemoryLimitMiB: 64, ProcessLimit: 8},
			{Name: config.ProfileNetworkObserver, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/safeterm/network.sock", RunAsUser: "test", MemoryLimitMiB: 64, ProcessLimit: 8},
		},
		Executables: []config.ExecutableDefinition{
			{ID: "uname", Path: "/usr/bin/uname", AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics}},
			{ID: "tcpdump", Path: "/usr/bin/tcpdump", AllowedProfiles: []config.RunnerProfile{config.ProfileNetworkObserver}},
		},
		Operations: []config.OperationDefinition{
			{Name: "system_info", RunnerProfile: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, ExecutableRef: "uname"},
			{Name: "trace", RunnerProfile: config.ProfileNetworkObserver, ExecutorKind: config.ExecutorProcess, ExecutableRef: "tcpdump"},
		},
		Roles: []config.RoleDefinition{{Name: "viewer"}, {Name: "netops"}, {Name: "unassigned"}},
		Policies: []config.PolicyDefinition{
			{Role: "viewer", Operations: []string{"system_info"}},
			{Role: "netops", Operations: []string{"system_info", "trace"}},
		},
	}
}

func requestWithRoles(t *testing.T, command string, roles []string) *contract.ExecutionRequest {
	t.Helper()
	req, err := contract.NewExecutionRequest(command, nil, contract.Metadata{ClaimedRoles: roles})
	if err != nil {
		t.Fatalf("failed to build test request: %v", err)
	}
	return req
}

// TEST-POLICY-001
// Requirement: REQ-005 (AC-1)
// Scenario: a request with no claimed roles at all must be denied
// (fail-closed), never implicitly allowed.
func TestAuthorize_NoClaimedRoles_Denied(t *testing.T) {
	e := New(testPolicyConfig())
	for _, roles := range [][]string{nil, {}} {
		if err := e.Authorize(requestWithRoles(t, "system_info", roles)); err == nil {
			t.Fatal("expected denial for request with no claimed roles, got nil")
		}
	}
}

// TEST-POLICY-002
// Requirement: REQ-005 (AC-2)
// Scenario: a role that IS permitted for the requested operation must be
// authorized.
func TestAuthorize_RoleWithPermission_Allowed(t *testing.T) {
	e := New(testPolicyConfig())
	req := requestWithRoles(t, "trace", []string{"netops"})

	if err := e.Authorize(req); err != nil {
		t.Fatalf("expected netops to be authorized for trace, got error: %v", err)
	}
}

// TEST-POLICY-003
// Requirement: REQ-005 (AC-2), mirrors the shipped policies.json exactly:
// "viewer" may run system_info but must NOT be allowed to run "trace".
func TestAuthorize_ViewerCannotRunTrace(t *testing.T) {
	e := New(testPolicyConfig())
	req := requestWithRoles(t, "trace", []string{"viewer"})

	if err := e.Authorize(req); err == nil {
		t.Fatal("expected viewer role to be denied for trace operation, got nil")
	}
}

// TEST-POLICY-004
// Requirement: REQ-005 (AC-4)
// Scenario: a role name that is not configured in policies.json at all
// (e.g. a typo, or a role that was removed) must be denied, not silently
// treated as having full or partial access.
func TestAuthorize_UnknownRole_Denied(t *testing.T) {
	e := New(testPolicyConfig())
	req := requestWithRoles(t, "system_info", []string{"ghost_role"})

	if err := e.Authorize(req); err == nil {
		t.Fatal("expected unknown role to be denied, got nil")
	}
}

// TEST-POLICY-005
// Requirement: REQ-005 (AC-3)
// Scenario: if a request carries multiple claimed roles and AT LEAST ONE
// of them grants the operation, the request must be authorized (additive
// RBAC semantics).
func TestAuthorize_MultipleRoles_OneGrantsAccess_Allowed(t *testing.T) {
	e := New(testPolicyConfig())
	req := requestWithRoles(t, "trace", []string{"viewer", "netops"})

	if err := e.Authorize(req); err != nil {
		t.Fatalf("expected access to be granted when at least one role permits it, got error: %v", err)
	}
}

// TEST-POLICY-006
// Requirement: REQ-005 (AC-3)
// Scenario: if none of several claimed roles grant the operation, the
// request must be denied.
func TestAuthorize_MultipleRoles_NoneGrantAccess_Denied(t *testing.T) {
	e := New(testPolicyConfig())
	req := requestWithRoles(t, "trace", []string{"viewer", "ghost_role"})

	if err := e.Authorize(req); err == nil {
		t.Fatal("expected denial when no claimed role grants the operation, got nil")
	}
}

// TEST-POLICY-007
// Requirement: REQ-005 (AC-2)
// Scenario: viewer running the operation it IS permitted for must succeed.
func TestAuthorize_ViewerCanRunSystemInfo(t *testing.T) {
	e := New(testPolicyConfig())
	req := requestWithRoles(t, "system_info", []string{"viewer"})

	if err := e.Authorize(req); err != nil {
		t.Fatalf("expected viewer to be authorized for system_info, got error: %v", err)
	}
}

// TEST-POLICY-008
// Requirement: REQ-005 (AC-3, AC-4)
// Unknown roles grant nothing and do not veto another role's explicit grant.
func TestAuthorize_RoleOrderAndDuplicates(t *testing.T) {
	e := New(testPolicyConfig())
	for _, roles := range [][]string{
		{"ghost", "netops"}, {"netops", "ghost"},
		{"netops", "netops"}, {"", "netops"},
	} {
		if err := e.Authorize(requestWithRoles(t, "trace", roles)); err != nil {
			t.Errorf("roles %v: expected authorization, got %v", roles, err)
		}
	}
	if err := e.Authorize(requestWithRoles(t, "system_info", []string{""})); err == nil {
		t.Fatal("an empty role must not grant access")
	}
}

// TEST-POLICY-009
// Requirement: REQ-005 (AC-5)
// Defined roles without grants, including empty policies, must fail closed.
func TestAuthorize_NoPolicyOrEmptyPolicy_Denied(t *testing.T) {
	for _, name := range []string{"missing policy", "empty policy", "no policies"} {
		t.Run(name, func(t *testing.T) {
			cfg := testPolicyConfig()
			switch name {
			case "empty policy":
				cfg.Policies = append(cfg.Policies, config.PolicyDefinition{Role: "unassigned"})
			case "no policies":
				cfg.Policies = nil
			}
			if err := config.ValidateConfig(cfg); err != nil {
				t.Fatal(err)
			}
			if err := New(cfg).Authorize(requestWithRoles(t, "system_info", []string{"unassigned"})); err == nil {
				t.Fatal("expected denial for role without grants")
			}
		})
	}
}

// TEST-POLICY-010
// Requirement: REQ-005 (AC-6)
// Grants from separate policies merge without leaking into other roles.
func TestAuthorize_MultiplePoliciesForRole_AreMerged(t *testing.T) {
	cfg := testPolicyConfig()
	cfg.Policies = []config.PolicyDefinition{
		{Role: "viewer", Operations: []string{"system_info"}},
		{Role: "netops", Operations: []string{"system_info"}},
		{Role: "netops", Operations: []string{"trace", "trace"}},
	}
	if err := config.ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	e := New(cfg)
	for _, operation := range []string{"system_info", "trace"} {
		if err := e.Authorize(requestWithRoles(t, operation, []string{"netops"})); err != nil {
			t.Errorf("merged grant missing for %s: %v", operation, err)
		}
	}
	if err := e.Authorize(requestWithRoles(t, "trace", []string{"viewer"})); err == nil {
		t.Fatal("netops grant leaked into viewer")
	}
}

// TEST-POLICY-011
// Requirement: REQ-005 (AC-7)
// Matching is by exact logical operation, never by raw executable or wildcard.
func TestAuthorize_UnknownOperation_Denied(t *testing.T) {
	e := New(testPolicyConfig())
	for _, operation := range []string{"missing", "tcpdump", "/usr/bin/uname", "*", "TRACE"} {
		if err := e.Authorize(requestWithRoles(t, operation, []string{"netops"})); err == nil {
			t.Errorf("unexpected authorization for %q", operation)
		}
	}
}

// TEST-POLICY-012
// Requirement: REQ-005 (AC-8)
// Direct calls with no request or configuration must not panic or grant access.
func TestAuthorize_NilInputsAndZeroValue_Denied(t *testing.T) {
	for _, e := range []*Enforcer{New(testPolicyConfig()), New(nil), {}} {
		if err := e.Authorize(nil); err == nil {
			t.Fatal("expected denial for nil request")
		}
	}
	for _, e := range []*Enforcer{New(nil), {}} {
		if err := e.Authorize(requestWithRoles(t, "trace", []string{"netops"})); err == nil {
			t.Fatal("unconfigured enforcer must not grant access")
		}
	}
}

// TEST-POLICY-013
// Requirement: REQ-005 (AC-2)
// Exercise the complete role/operation matrix against the shipped config.
func TestAuthorize_ShippedConfig(t *testing.T) {
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatal(err)
	}
	e := New(cfg)
	for _, tc := range []struct {
		role, operation string
		allowed         bool
	}{
		{"viewer", "system_info", true}, {"viewer", "trace", false},
		{"netops", "system_info", true}, {"netops", "trace", true},
	} {
		if err := e.Authorize(requestWithRoles(t, tc.operation, []string{tc.role})); (err == nil) != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v, error=%v", tc.role, tc.operation, tc.allowed, err)
		}
	}
}

// TEST-POLICY-014
// Requirement: REQ-005 (AC-6)
// Mutating the source config must not silently alter an existing grant table.
func TestAuthorize_GrantTableIsIndependentOfSourceConfig(t *testing.T) {
	cfg := testPolicyConfig()
	e := New(cfg)
	cfg.Policies[0].Operations[0] = "trace"
	if err := e.Authorize(requestWithRoles(t, "trace", []string{"viewer"})); err == nil {
		t.Fatal("source config mutation granted new access")
	}
	if err := e.Authorize(requestWithRoles(t, "system_info", []string{"viewer"})); err != nil {
		t.Fatalf("source config mutation removed original grant: %v", err)
	}
}
