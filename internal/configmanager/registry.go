package configmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const maxRegistryBytes = 4 << 20

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)
var fieldIdentifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)

func LoadRegistry(path string) (*Registry, error) {
	data, info, err := readManagedFile(path)
	if err != nil {
		return nil, fmt.Errorf("configmanager: securely read registry: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxRegistryBytes {
		return nil, fmt.Errorf("configmanager: registry must be a regular file no larger than %d bytes", maxRegistryBytes)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("configmanager: registry must not be group- or world-writable")
	}
	if runtime.GOOS != "windows" {
		uid, ok := fileOwnerUID(info)
		if !ok || uid != 0 {
			return nil, fmt.Errorf("configmanager: registry must be owned by root")
		}
	}
	var registry Registry
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return nil, fmt.Errorf("configmanager: decode registry: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("configmanager: registry has trailing data")
	}
	if err := ValidateRegistry(&registry); err != nil {
		return nil, err
	}
	return &registry, nil
}

func ValidateRegistry(registry *Registry) error {
	if registry == nil || strings.TrimSpace(registry.Version) == "" || len(registry.Resources) == 0 {
		return fmt.Errorf("configmanager: registry version and resources are required")
	}
	ids := map[string]bool{}
	paths := map[string]bool{}
	for index := range registry.Resources {
		resource := &registry.Resources[index]
		if !identifierPattern.MatchString(resource.ID) || ids[resource.ID] {
			return fmt.Errorf("configmanager: invalid or duplicate resource id %q", resource.ID)
		}
		ids[resource.ID] = true
		if !identifierPattern.MatchString(resource.Profile) {
			return fmt.Errorf("configmanager: resource %q has invalid profile", resource.ID)
		}
		if !filepath.IsAbs(resource.Path) || filepath.Clean(resource.Path) != resource.Path || paths[resource.Path] {
			return fmt.Errorf("configmanager: resource %q path must be absolute, clean, and unique", resource.ID)
		}
		paths[resource.Path] = true
		if resource.Format != FormatINI && resource.Format != FormatJSON {
			return fmt.Errorf("configmanager: resource %q has unsupported format %q", resource.ID, resource.Format)
		}
		if resource.Access != AccessReadOnly && resource.Access != AccessReadWrite {
			return fmt.Errorf("configmanager: resource %q has unsupported access %q", resource.ID, resource.Access)
		}
		if len(resource.Fields) == 0 || len(resource.Permissions.ReadRoles) == 0 {
			return fmt.Errorf("configmanager: resource %q requires fields and read roles", resource.ID)
		}
		if err := validatePermissionRoles(resource.ID, resource.Permissions); err != nil {
			return err
		}
		if resource.Access == AccessReadOnly && (len(resource.Permissions.ProposeRoles) != 0 || len(resource.Permissions.ApplyRoles) != 0 || len(resource.Permissions.RollbackRoles) != 0) {
			return fmt.Errorf("configmanager: read-only resource %q grants mutation roles", resource.ID)
		}
		if resource.Access == AccessReadWrite && (len(resource.Permissions.ProposeRoles) == 0 || len(resource.Permissions.ApplyRoles) == 0) {
			return fmt.Errorf("configmanager: writable resource %q requires propose and apply roles", resource.ID)
		}
		if resource.Backup.Enabled && (resource.Backup.Keep < 1 || resource.Backup.Keep > 32) {
			return fmt.Errorf("configmanager: resource %q backup retention must be between 1 and 32", resource.ID)
		}
		if !resource.Backup.Enabled && resource.Backup.Keep != 0 {
			return fmt.Errorf("configmanager: resource %q disabled backup must have zero retention", resource.ID)
		}
		if !resource.Backup.Enabled && (len(resource.Permissions.RollbackRoles) != 0 || resource.RollbackOnFailure) {
			return fmt.Errorf("configmanager: resource %q rollback requires backups", resource.ID)
		}
		if resource.ActivationOperation == "" && (resource.HealthCheckOperation != "" || resource.RollbackOnFailure || len(resource.Permissions.ActivateRoles) != 0) {
			return fmt.Errorf("configmanager: resource %q activation settings require an activation operation", resource.ID)
		}
		if resource.ActivationOperation != "" && (!identifierPattern.MatchString(resource.ActivationOperation) || len(resource.Permissions.ActivateRoles) == 0) {
			return fmt.Errorf("configmanager: resource %q activation requires a typed operation and roles", resource.ID)
		}
		if resource.HealthCheckOperation != "" && !identifierPattern.MatchString(resource.HealthCheckOperation) {
			return fmt.Errorf("configmanager: resource %q has invalid health-check operation", resource.ID)
		}
		if err := validateFields(*resource); err != nil {
			return err
		}
		if err := validateCrossRules(*resource); err != nil {
			return err
		}
	}
	return nil
}

func validatePermissionRoles(resourceID string, permissions Permissions) error {
	groups := [][]string{permissions.ReadRoles, permissions.ProposeRoles, permissions.ApplyRoles, permissions.ActivateRoles, permissions.RollbackRoles}
	for _, roles := range groups {
		seen := map[string]bool{}
		for _, role := range roles {
			if !identifierPattern.MatchString(role) || seen[role] {
				return fmt.Errorf("configmanager: resource %q has invalid or duplicate permission role %q", resourceID, role)
			}
			seen[role] = true
		}
	}
	return nil
}

func validateFields(resource Resource) error {
	seen := map[string]bool{}
	for index := range resource.Fields {
		field := &resource.Fields[index]
		if !fieldIdentifierPattern.MatchString(field.Key) || (field.Section != "" && !fieldIdentifierPattern.MatchString(field.Section)) {
			return fmt.Errorf("configmanager: resource %q has invalid field %q.%q", resource.ID, field.Section, field.Key)
		}
		coordinate := field.Section + "\x00" + field.Key
		if seen[coordinate] {
			return fmt.Errorf("configmanager: resource %q has duplicate field %q.%q", resource.ID, field.Section, field.Key)
		}
		seen[coordinate] = true
		switch field.Type {
		case FieldString:
			if len(field.Enum) != 0 || field.Min != nil || field.Max != nil || field.Path != nil {
				return invalidField(resource, *field)
			}
		case FieldInteger:
			if len(field.Enum) != 0 || field.MinLength != nil || field.MaxLength != nil || field.Pattern != "" || field.Path != nil || (field.Min != nil && field.Max != nil && *field.Min > *field.Max) {
				return invalidField(resource, *field)
			}
		case FieldBoolean:
			if len(field.Enum) != 0 || field.Min != nil || field.Max != nil || field.MinLength != nil || field.MaxLength != nil || field.Pattern != "" || field.Path != nil {
				return invalidField(resource, *field)
			}
		case FieldEnum:
			if len(field.Enum) == 0 || field.Min != nil || field.Max != nil || field.Path != nil {
				return invalidField(resource, *field)
			}
			values := map[string]bool{}
			for _, value := range field.Enum {
				if value == "" || values[value] {
					return invalidField(resource, *field)
				}
				values[value] = true
			}
		case FieldPath:
			if field.Path == nil || len(field.Path.AllowedPrefixes) == 0 || len(field.Enum) != 0 || field.Min != nil || field.Max != nil {
				return invalidField(resource, *field)
			}
			for _, prefix := range field.Path.AllowedPrefixes {
				if !filepath.IsAbs(prefix) || filepath.Clean(prefix) != prefix {
					return invalidField(resource, *field)
				}
			}
			if field.Path.FileType != "" && field.Path.FileType != "file" && field.Path.FileType != "directory" {
				return invalidField(resource, *field)
			}
			if field.Path.ExpectedMode != "" {
				if _, err := strconv.ParseUint(field.Path.ExpectedMode, 8, 12); err != nil {
					return invalidField(resource, *field)
				}
			}
		default:
			return invalidField(resource, *field)
		}
		if field.MinLength != nil && (*field.MinLength < 0 || field.MaxLength != nil && *field.MinLength > *field.MaxLength) {
			return invalidField(resource, *field)
		}
		if field.MaxLength != nil && *field.MaxLength < 0 {
			return invalidField(resource, *field)
		}
		if field.Pattern != "" {
			if !strings.HasPrefix(field.Pattern, "^") || !strings.HasSuffix(field.Pattern, "$") {
				return fmt.Errorf("configmanager: resource %q field %q.%q pattern must be anchored", resource.ID, field.Section, field.Key)
			}
			if _, err := regexp.Compile(field.Pattern); err != nil {
				return fmt.Errorf("configmanager: resource %q field %q.%q has invalid pattern", resource.ID, field.Section, field.Key)
			}
		}
	}
	return nil
}

func validateCrossRules(resource Resource) error {
	fields := map[string]bool{}
	for _, field := range resource.Fields {
		fields[field.Section+"\x00"+field.Key] = true
	}
	for _, rule := range resource.CrossFieldRules {
		if rule.Type != "not_equal" && rule.Type != "less_or_equal" {
			return fmt.Errorf("configmanager: resource %q has unsupported cross-field rule %q", resource.ID, rule.Type)
		}
		if !fields[rule.Section+"\x00"+rule.Key] || !fields[rule.OtherSection+"\x00"+rule.OtherKey] {
			return fmt.Errorf("configmanager: resource %q cross-field rule references unknown field", resource.ID)
		}
	}
	return nil
}

func invalidField(resource Resource, field FieldSchema) error {
	return fmt.Errorf("configmanager: resource %q field %q.%q has inconsistent schema", resource.ID, field.Section, field.Key)
}

func resourcesForProfile(registry *Registry, profile string) []Resource {
	result := []Resource{}
	for _, resource := range registry.Resources {
		if resource.Profile == profile {
			result = append(result, resource)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
