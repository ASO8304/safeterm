package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	namePattern       = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	profilePattern    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	capabilityPattern = regexp.MustCompile(`^CAP_[A-Z0-9_]+$`)
	unitPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]*\.service$`)
)

// ValidateConfig rejects incomplete or cross-profile catalog snapshots before
// any runtime component can observe them.
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config: nil configuration")
	}
	if cfg.Version == "" {
		return fmt.Errorf("config: catalog version must not be empty")
	}
	profiles, err := validateProfiles(cfg.Profiles)
	if err != nil {
		return err
	}
	executables, err := validateExecutables(cfg.Executables, profiles)
	if err != nil {
		return err
	}
	opNames, err := validateOperations(cfg.Operations, profiles, executables)
	if err != nil {
		return err
	}
	roleNames, err := validateRoles(cfg.Roles)
	if err != nil {
		return err
	}
	return validatePolicies(cfg.Policies, opNames, roleNames)
}

func validateProfiles(items []ProfileDefinition) (map[RunnerProfile]ProfileDefinition, error) {
	profiles := make(map[RunnerProfile]ProfileDefinition, len(items))
	for _, profile := range items {
		if !profilePattern.MatchString(string(profile.Name)) {
			return nil, fmt.Errorf("config: invalid runner profile name %q", profile.Name)
		}
		if _, exists := profiles[profile.Name]; exists {
			return nil, fmt.Errorf("config: duplicate runner profile %q", profile.Name)
		}
		if profile.ExecutorKind != ExecutorProcess && profile.ExecutorKind != ExecutorSystemd {
			return nil, fmt.Errorf("config: profile %q: unsupported executor kind %q", profile.Name, profile.ExecutorKind)
		}
		if !filepath.IsAbs(profile.SocketPath) {
			return nil, fmt.Errorf("config: profile %q: socket_path must be absolute", profile.Name)
		}
		if profile.RunAsUser == "" || profile.MemoryLimitMiB <= 0 || profile.ProcessLimit <= 0 {
			return nil, fmt.Errorf("config: profile %q: run_as_user and positive resource limits are required", profile.Name)
		}
		seenCapabilities := map[string]bool{}
		for _, capability := range profile.Capabilities {
			if !capabilityPattern.MatchString(capability) || capability == "CAP_SYS_ADMIN" || seenCapabilities[capability] {
				return nil, fmt.Errorf("config: profile %q: invalid or duplicate capability %q", profile.Name, capability)
			}
			seenCapabilities[capability] = true
		}
		profiles[profile.Name] = profile
	}
	return profiles, nil
}

func validateExecutables(items []ExecutableDefinition, profiles map[RunnerProfile]ProfileDefinition) (map[string]ExecutableDefinition, error) {
	executables := make(map[string]ExecutableDefinition, len(items))
	for _, executable := range items {
		if !namePattern.MatchString(executable.ID) {
			return nil, fmt.Errorf("config: invalid executable id %q", executable.ID)
		}
		if _, exists := executables[executable.ID]; exists {
			return nil, fmt.Errorf("config: duplicate executable id %q", executable.ID)
		}
		if !filepath.IsAbs(executable.Path) {
			return nil, fmt.Errorf("config: executable %q: path must be absolute", executable.ID)
		}
		if len(executable.AllowedProfiles) == 0 {
			return nil, fmt.Errorf("config: executable %q: at least one allowed profile is required", executable.ID)
		}
		seenProfiles := map[RunnerProfile]bool{}
		for _, profile := range executable.AllowedProfiles {
			if _, exists := profiles[profile]; !exists || seenProfiles[profile] {
				return nil, fmt.Errorf("config: executable %q: unknown or duplicate allowed profile %q", executable.ID, profile)
			}
			seenProfiles[profile] = true
		}
		executables[executable.ID] = executable
	}
	return executables, nil
}

func validateOperations(ops []OperationDefinition, profiles map[RunnerProfile]ProfileDefinition, executables map[string]ExecutableDefinition) (map[string]bool, error) {
	names := map[string]bool{}
	for _, op := range ops {
		if !namePattern.MatchString(op.Name) {
			return nil, fmt.Errorf("config: invalid operation name %q", op.Name)
		}
		if names[op.Name] {
			return nil, fmt.Errorf("config: duplicate operation name: %q", op.Name)
		}
		names[op.Name] = true
		profile, exists := profiles[op.RunnerProfile]
		if !exists {
			return nil, fmt.Errorf("config: operation %q: unknown runner profile %q", op.Name, op.RunnerProfile)
		}
		if op.ExecutorKind != profile.ExecutorKind {
			return nil, fmt.Errorf("config: operation %q: executor kind %q conflicts with profile %q kind %q", op.Name, op.ExecutorKind, op.RunnerProfile, profile.ExecutorKind)
		}
		switch op.ExecutorKind {
		case ExecutorProcess:
			if op.ExecutableRef == "" || op.Systemd != nil {
				return nil, fmt.Errorf("config: process operation %q requires executable_ref and no systemd definition", op.Name)
			}
			executable, exists := executables[op.ExecutableRef]
			if !exists || !containsProfile(executable.AllowedProfiles, op.RunnerProfile) {
				return nil, fmt.Errorf("config: operation %q: executable %q is not approved for profile %q", op.Name, op.ExecutableRef, op.RunnerProfile)
			}
		case ExecutorSystemd:
			if op.ExecutableRef != "" || len(op.ArgsTemplate) != 0 || op.Systemd == nil {
				return nil, fmt.Errorf("config: systemd operation %q requires only a typed systemd definition", op.Name)
			}
			if len(op.Parameters) != 0 || !unitPattern.MatchString(op.Systemd.Unit) || strings.Contains(op.Systemd.Unit, "..") || !validSystemdAction(op.Systemd.Action) {
				return nil, fmt.Errorf("config: systemd operation %q has invalid unit or action", op.Name)
			}
		default:
			return nil, fmt.Errorf("config: operation %q: unsupported executor kind %q", op.Name, op.ExecutorKind)
		}
		for _, parameter := range op.Parameters {
			if err := validateParameterDefinition(op.Name, parameter); err != nil {
				return nil, err
			}
		}
	}
	return names, nil
}

func containsProfile(items []RunnerProfile, wanted RunnerProfile) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}

func validSystemdAction(action string) bool {
	switch action {
	case "start", "stop", "restart", "status":
		return true
	default:
		return false
	}
}

func validateParameterDefinition(opName string, parameter ParameterDefinition) error {
	if parameter.Name == "" {
		return fmt.Errorf("config: operation %q: a parameter has an empty name", opName)
	}
	switch parameter.Type {
	case ParamTypeInt:
		if parameter.Pattern != "" {
			return fmt.Errorf("config: operation %q: parameter %q: pattern is not valid for type %q", opName, parameter.Name, ParamTypeInt)
		}
		if parameter.Min != nil && parameter.Max != nil && *parameter.Min > *parameter.Max {
			return fmt.Errorf("config: operation %q: parameter %q: min (%d) is greater than max (%d)", opName, parameter.Name, *parameter.Min, *parameter.Max)
		}
	case ParamTypeString:
		if parameter.Min != nil || parameter.Max != nil {
			return fmt.Errorf("config: operation %q: parameter %q: min/max is not valid for type %q", opName, parameter.Name, ParamTypeString)
		}
	default:
		return fmt.Errorf("config: operation %q: parameter %q: unknown type %q", opName, parameter.Name, parameter.Type)
	}
	return nil
}

func validateRoles(roles []RoleDefinition) (map[string]bool, error) {
	names := map[string]bool{}
	for _, role := range roles {
		if role.Name == "" {
			return nil, fmt.Errorf("config: a role has an empty name")
		}
		if names[role.Name] {
			return nil, fmt.Errorf("config: duplicate role name: %q", role.Name)
		}
		names[role.Name] = true
	}
	return names, nil
}

func validatePolicies(policies []PolicyDefinition, opNames, roleNames map[string]bool) error {
	for _, policy := range policies {
		if !roleNames[policy.Role] {
			return fmt.Errorf("config: policy references unknown role: %q", policy.Role)
		}
		for _, operation := range policy.Operations {
			if !opNames[operation] {
				return fmt.Errorf("config: policy for role %q references unknown operation: %q", policy.Role, operation)
			}
		}
	}
	return nil
}
