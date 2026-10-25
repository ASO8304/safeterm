package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	namePattern       = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	aliasPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]*$`)
	profilePattern    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	capabilityPattern = regexp.MustCompile(`^CAP_[A-Z0-9_]+$`)
	unitPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]*\.service$`)
	sha256Pattern     = regexp.MustCompile(`^[a-f0-9]{64}$`)
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
	if err := validateProfileExecutables(profiles, executables); err != nil {
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
		if profile.ExecutorKind == ExecutorProcess && profile.ControllerExecutableRef != "" {
			return nil, fmt.Errorf("config: process profile %q must not define a controller executable", profile.Name)
		}
		if profile.ExecutorKind == ExecutorSystemd && profile.ControllerExecutableRef == "" {
			return nil, fmt.Errorf("config: systemd profile %q requires a controller executable", profile.Name)
		}
		if profile.ExecutorKind == ExecutorProcess && len(profile.ControllerArgsPrefix) != 0 {
			return nil, fmt.Errorf("config: process profile %q must not define controller arguments", profile.Name)
		}
		for _, argument := range profile.ControllerArgsPrefix {
			if argument == "" || strings.ContainsRune(argument, '\x00') {
				return nil, fmt.Errorf("config: profile %q has an invalid controller argument", profile.Name)
			}
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
	paths := make(map[string]bool, len(items))
	for _, executable := range items {
		if !namePattern.MatchString(executable.ID) {
			return nil, fmt.Errorf("config: invalid executable id %q", executable.ID)
		}
		if _, exists := executables[executable.ID]; exists {
			return nil, fmt.Errorf("config: duplicate executable id %q", executable.ID)
		}
		if !filepath.IsAbs(executable.Path) || filepath.Clean(executable.Path) != executable.Path {
			return nil, fmt.Errorf("config: executable %q: path must be absolute and clean", executable.ID)
		}
		if paths[executable.Path] {
			return nil, fmt.Errorf("config: executable %q: duplicate path %q", executable.ID, executable.Path)
		}
		paths[executable.Path] = true
		if !sha256Pattern.MatchString(executable.SHA256) {
			return nil, fmt.Errorf("config: executable %q: sha256 must be 64 lowercase hexadecimal characters", executable.ID)
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

func validateProfileExecutables(profiles map[RunnerProfile]ProfileDefinition, executables map[string]ExecutableDefinition) error {
	for _, profile := range profiles {
		if profile.ExecutorKind != ExecutorSystemd {
			continue
		}
		executable, exists := executables[profile.ControllerExecutableRef]
		if !exists || !containsProfile(executable.AllowedProfiles, profile.Name) {
			return fmt.Errorf("config: profile %q: controller executable %q is not approved for this profile", profile.Name, profile.ControllerExecutableRef)
		}
		if len(profile.ControllerArgsPrefix) != 0 {
			expected := []string{"-n", "/usr/bin/systemctl", "--no-pager"}
			if executable.Path != "/usr/bin/sudo" || len(profile.ControllerArgsPrefix) != len(expected) {
				return fmt.Errorf("config: profile %q has an unsupported privileged controller", profile.Name)
			}
			for index := range expected {
				if profile.ControllerArgsPrefix[index] != expected[index] {
					return fmt.Errorf("config: profile %q has an unsupported privileged controller", profile.Name)
				}
			}
			systemctl, ok := executables["systemctl"]
			if !ok || systemctl.Path != "/usr/bin/systemctl" || !containsProfile(systemctl.AllowedProfiles, profile.Name) {
				return fmt.Errorf("config: profile %q privileged controller requires an approved systemctl executable", profile.Name)
			}
		}
	}
	return nil
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
			if op.LongRunning {
				return nil, fmt.Errorf("config: systemd operation %q cannot be long-running", op.Name)
			}
			if op.ExecutableRef != "" || len(op.ArgsTemplate) != 0 || len(op.ArgumentGroups) != 0 || len(op.RequiredAny) != 0 || len(op.ArgumentGroupSeparator) != 0 || op.Systemd == nil {
				return nil, fmt.Errorf("config: systemd operation %q requires only a typed systemd definition", op.Name)
			}
			if len(op.Parameters) != 0 {
				return nil, fmt.Errorf("config: systemd operation %q cannot accept parameters", op.Name)
			}
			if err := ValidateSystemdDefinition(op.Systemd); err != nil {
				return nil, fmt.Errorf("config: systemd operation %q: %w", op.Name, err)
			}
		default:
			return nil, fmt.Errorf("config: operation %q: unsupported executor kind %q", op.Name, op.ExecutorKind)
		}
		for _, parameter := range op.Parameters {
			if err := validateParameterDefinition(op.Name, parameter); err != nil {
				return nil, err
			}
		}
		defined := map[string]bool{}
		for _, p := range op.Parameters {
			defined[p.Name] = true
		}
		seenRequired := map[string]bool{}
		for _, name := range op.RequiredAny {
			if !defined[name] || seenRequired[name] {
				return nil, fmt.Errorf("config: operation %q: invalid required_any parameter %q", op.Name, name)
			}
			seenRequired[name] = true
		}
		for _, group := range op.ArgumentGroups {
			if !defined[group.When] || len(group.ArgsTemplate) == 0 {
				return nil, fmt.Errorf("config: operation %q: invalid argument group %q", op.Name, group.When)
			}
		}
		for _, token := range op.ArgumentGroupSeparator {
			if token == "" || strings.ContainsAny(token, "{}\x00") {
				return nil, fmt.Errorf("config: operation %q: invalid argument group separator", op.Name)
			}
		}
		if len(op.ArgumentGroups) == 0 && len(op.ArgumentGroupSeparator) != 0 {
			return nil, fmt.Errorf("config: operation %q: separator requires argument groups", op.Name)
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
	case "start", "stop", "restart", "reload", "enable", "disable", "status":
		return true
	default:
		return false
	}
}

func validateParameterDefinition(opName string, parameter ParameterDefinition) error {
	if parameter.Name == "" {
		return fmt.Errorf("config: operation %q: a parameter has an empty name", opName)
	}
	if parameter.Required && parameter.Default != "" {
		return fmt.Errorf("config: operation %q: required parameter %q cannot define a default", opName, parameter.Name)
	}
	seenAliases := map[string]bool{}
	for _, alias := range parameter.Aliases {
		if !regexp.MustCompile(`^-[A-Za-z][A-Za-z0-9-]*$`).MatchString(alias) || seenAliases[alias] {
			return fmt.Errorf("config: operation %q: parameter %q has invalid or duplicate alias %q", opName, parameter.Name, alias)
		}
		seenAliases[alias] = true
	}
	switch parameter.Type {
	case ParamTypeInt:
		if parameter.Pattern != "" || len(parameter.ValueMap) != 0 {
			return fmt.Errorf("config: operation %q: parameter %q: pattern is not valid for type %q", opName, parameter.Name, ParamTypeInt)
		}
		if parameter.Min != nil && parameter.Max != nil && *parameter.Min > *parameter.Max {
			return fmt.Errorf("config: operation %q: parameter %q: min (%d) is greater than max (%d)", opName, parameter.Name, *parameter.Min, *parameter.Max)
		}
		if parameter.Default != "" {
			value, err := strconv.Atoi(parameter.Default)
			if err != nil || parameter.Min != nil && value < *parameter.Min || parameter.Max != nil && value > *parameter.Max {
				return fmt.Errorf("config: operation %q: parameter %q has an invalid default", opName, parameter.Name)
			}
		}
	case ParamTypeString:
		if parameter.Min != nil || parameter.Max != nil {
			return fmt.Errorf("config: operation %q: parameter %q: min/max is not valid for type %q", opName, parameter.Name, ParamTypeString)
		}
		if parameter.Default != "" {
			if strings.ContainsAny(parameter.Default, ";&|`$(){}<>\n\r'\"") {
				return fmt.Errorf("config: operation %q: parameter %q default contains forbidden characters", opName, parameter.Name)
			}
			if parameter.Pattern != "" {
				matched, err := regexp.MatchString(parameter.Pattern, parameter.Default)
				if err != nil || !matched {
					return fmt.Errorf("config: operation %q: parameter %q has an invalid default", opName, parameter.Name)
				}
			}
			if len(parameter.ValueMap) != 0 {
				if _, ok := parameter.ValueMap[parameter.Default]; !ok {
					return fmt.Errorf("config: operation %q: parameter %q default is not allowed", opName, parameter.Name)
				}
			}
		}
	default:
		return fmt.Errorf("config: operation %q: parameter %q: unknown type %q", opName, parameter.Name, parameter.Type)
	}
	if len(parameter.ValueMap) != 0 {
		re, err := regexp.Compile(parameter.Pattern)
		if err != nil {
			return fmt.Errorf("config: operation %q: parameter %q has invalid pattern: %w", opName, parameter.Name, err)
		}
		for alias, value := range parameter.ValueMap {
			if !aliasPattern.MatchString(alias) || value == "" || strings.ContainsRune(value, '\x00') {
				return fmt.Errorf("config: operation %q: parameter %q has invalid mapped value %q", opName, parameter.Name, alias)
			}
			if parameter.Pattern != "" {
				location := re.FindStringIndex(alias)
				if location == nil || location[0] != 0 || location[1] != len(alias) {
					return fmt.Errorf("config: operation %q: mapped alias %q does not match parameter %q pattern", opName, alias, parameter.Name)
				}
			}
		}
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
