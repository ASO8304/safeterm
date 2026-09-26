package config

import "fmt"

// ValidateConfig performs structural / referential-integrity checks on a
// loaded Config. This is deliberately NOT the same thing as the Validation
// Engine (M4): M4 will validate a *user's request arguments* at runtime
// against an operation's ParameterDefinitions; this function validates that
// the *configuration itself* is well-formed, so a broken or inconsistent
// config file fails fast at startup, instead of causing confusing behavior
// later when a real user hits it (see REQ-003, AC-1..AC-6).
func ValidateConfig(cfg *Config) error {
	opNames, err := validateOperations(cfg.Operations)
	if err != nil {
		return err
	}

	roleNames, err := validateRoles(cfg.Roles)
	if err != nil {
		return err
	}

	return validatePolicies(cfg.Policies, opNames, roleNames)
}

func validateOperations(ops []OperationDefinition) (map[string]bool, error) {
	names := map[string]bool{}
	for _, op := range ops {
		if op.Name == "" {
			return nil, fmt.Errorf("config: an operation has an empty name")
		}
		if names[op.Name] {
			return nil, fmt.Errorf("config: duplicate operation name: %q", op.Name)
		}
		names[op.Name] = true

		if op.Command == "" {
			return nil, fmt.Errorf("config: operation %q: command must not be empty", op.Name)
		}

		for _, p := range op.Parameters {
			if err := validateParameterDefinition(op.Name, p); err != nil {
				return nil, err
			}
		}
	}
	return names, nil
}

func validateParameterDefinition(opName string, p ParameterDefinition) error {
	if p.Name == "" {
		return fmt.Errorf("config: operation %q: a parameter has an empty name", opName)
	}

	switch p.Type {
	case ParamTypeInt:
		if p.Pattern != "" {
			return fmt.Errorf("config: operation %q: parameter %q: pattern is not valid for type %q", opName, p.Name, ParamTypeInt)
		}
		if p.Min != nil && p.Max != nil && *p.Min > *p.Max {
			return fmt.Errorf("config: operation %q: parameter %q: min (%d) is greater than max (%d)", opName, p.Name, *p.Min, *p.Max)
		}
	case ParamTypeString:
		if p.Min != nil || p.Max != nil {
			return fmt.Errorf("config: operation %q: parameter %q: min/max is not valid for type %q", opName, p.Name, ParamTypeString)
		}
	default:
		return fmt.Errorf("config: operation %q: parameter %q: unknown type %q", opName, p.Name, p.Type)
	}
	return nil
}

func validateRoles(roles []RoleDefinition) (map[string]bool, error) {
	names := map[string]bool{}
	for _, r := range roles {
		if r.Name == "" {
			return nil, fmt.Errorf("config: a role has an empty name")
		}
		if names[r.Name] {
			return nil, fmt.Errorf("config: duplicate role name: %q", r.Name)
		}
		names[r.Name] = true
	}
	return names, nil
}

func validatePolicies(policies []PolicyDefinition, opNames, roleNames map[string]bool) error {
	for _, pol := range policies {
		if !roleNames[pol.Role] {
			return fmt.Errorf("config: policy references unknown role: %q", pol.Role)
		}
		for _, opName := range pol.Operations {
			if !opNames[opName] {
				return fmt.Errorf("config: policy for role %q references unknown operation: %q", pol.Role, opName)
			}
		}
	}
	return nil
}
