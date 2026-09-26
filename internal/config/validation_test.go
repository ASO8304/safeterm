package config

import "testing"

func intPtr(v int) *int { return &v }

func validOperation() OperationDefinition {
	return OperationDefinition{
		Name:    "system_info",
		Command: "uname",
	}
}

// TEST-CONFIG-001
// Requirement: REQ-003 (AC-1)
// Scenario: a well-formed config with no cross-references must pass.
func TestValidateConfig_ValidMinimalConfig_Passes(t *testing.T) {
	cfg := &Config{
		Operations: []OperationDefinition{validOperation()},
		Roles:      []RoleDefinition{{Name: "viewer"}},
		Policies:   []PolicyDefinition{{Role: "viewer", Operations: []string{"system_info"}}},
	}

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected valid config to pass, got error: %v", err)
	}
}

// TEST-CONFIG-002
// Requirement: REQ-003 (AC-2)
// Scenario: two operations with the same name must be rejected.
func TestValidateConfig_DuplicateOperationName_Rejected(t *testing.T) {
	cfg := &Config{
		Operations: []OperationDefinition{validOperation(), validOperation()},
	}

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for duplicate operation name, got nil")
	}
}

// TEST-CONFIG-003
// Requirement: REQ-003 (AC-3)
// Scenario: an operation with an empty command must be rejected, since
// Translation (M6) would have nothing to map the operation to.
func TestValidateConfig_OperationWithEmptyCommand_Rejected(t *testing.T) {
	cfg := &Config{
		Operations: []OperationDefinition{{Name: "broken"}},
	}

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for operation with empty command, got nil")
	}
}

// TEST-CONFIG-004
// Scenario: two roles with the same name must be rejected.
func TestValidateConfig_DuplicateRoleName_Rejected(t *testing.T) {
	cfg := &Config{
		Roles: []RoleDefinition{{Name: "viewer"}, {Name: "viewer"}},
	}

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for duplicate role name, got nil")
	}
}

// TEST-CONFIG-005
// Requirement: REQ-003 (AC-4)
// Scenario: a policy referencing a role that does not exist must be
// rejected, so a typo in roles.json never silently grants zero access
// instead of failing loudly.
func TestValidateConfig_PolicyReferencesUnknownRole_Rejected(t *testing.T) {
	cfg := &Config{
		Operations: []OperationDefinition{validOperation()},
		Roles:      []RoleDefinition{{Name: "viewer"}},
		Policies:   []PolicyDefinition{{Role: "ghost_role", Operations: []string{"system_info"}}},
	}

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for policy referencing unknown role, got nil")
	}
}

// TEST-CONFIG-006
// Requirement: REQ-003 (AC-5)
// Scenario: a policy referencing an operation that does not exist must be
// rejected.
func TestValidateConfig_PolicyReferencesUnknownOperation_Rejected(t *testing.T) {
	cfg := &Config{
		Roles:    []RoleDefinition{{Name: "viewer"}},
		Policies: []PolicyDefinition{{Role: "viewer", Operations: []string{"ghost_operation"}}},
	}

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for policy referencing unknown operation, got nil")
	}
}

// TEST-CONFIG-007
// Requirement: REQ-003 (AC-6)
// Scenario: an int parameter must not have a regex Pattern set — mixing
// constraints from two different types signals a config authoring mistake.
func TestValidateConfig_IntParameterWithPattern_Rejected(t *testing.T) {
	op := validOperation()
	op.Parameters = []ParameterDefinition{{Name: "port", Type: ParamTypeInt, Pattern: "^[0-9]+$"}}
	cfg := &Config{Operations: []OperationDefinition{op}}

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for int parameter with pattern set, got nil")
	}
}

// TEST-CONFIG-008
// Scenario: min must not be greater than max for an int parameter.
func TestValidateConfig_MinGreaterThanMax_Rejected(t *testing.T) {
	op := validOperation()
	op.Parameters = []ParameterDefinition{{Name: "port", Type: ParamTypeInt, Min: intPtr(100), Max: intPtr(1)}}
	cfg := &Config{Operations: []OperationDefinition{op}}

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for min > max, got nil")
	}
}

// TEST-CONFIG-009
// Scenario: a string parameter must not have Min/Max set.
func TestValidateConfig_StringParameterWithMinMax_Rejected(t *testing.T) {
	op := validOperation()
	op.Parameters = []ParameterDefinition{{Name: "host", Type: ParamTypeString, Min: intPtr(1)}}
	cfg := &Config{Operations: []OperationDefinition{op}}

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for string parameter with min/max set, got nil")
	}
}

// TEST-CONFIG-010
// Scenario: an unrecognized parameter type must be rejected outright,
// rather than silently treated as unconstrained.
func TestValidateConfig_UnknownParameterType_Rejected(t *testing.T) {
	op := validOperation()
	op.Parameters = []ParameterDefinition{{Name: "weird", Type: "float"}}
	cfg := &Config{Operations: []OperationDefinition{op}}

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for unknown parameter type, got nil")
	}
}
