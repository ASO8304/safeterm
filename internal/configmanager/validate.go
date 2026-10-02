package configmanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func normalizeValue(field FieldSchema, candidate string) (any, string, error) {
	if field.Required && candidate == "" {
		return nil, "", fmt.Errorf("configmanager: field %q.%q is required", field.Section, field.Key)
	}
	switch field.Type {
	case FieldInteger:
		value, err := strconv.ParseInt(strings.TrimSpace(candidate), 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("configmanager: field %q.%q must be an integer", field.Section, field.Key)
		}
		if field.Min != nil && value < *field.Min || field.Max != nil && value > *field.Max {
			return nil, "", fmt.Errorf("configmanager: field %q.%q is outside its allowed range", field.Section, field.Key)
		}
		return value, strconv.FormatInt(value, 10), nil
	case FieldBoolean:
		normalized := strings.ToLower(strings.TrimSpace(candidate))
		value, err := strconv.ParseBool(normalized)
		if err != nil || normalized != "true" && normalized != "false" {
			return nil, "", fmt.Errorf("configmanager: field %q.%q must be true or false", field.Section, field.Key)
		}
		return value, strconv.FormatBool(value), nil
	case FieldEnum:
		for _, allowed := range field.Enum {
			if candidate == allowed {
				return candidate, candidate, validateTextConstraints(field, candidate)
			}
		}
		return nil, "", fmt.Errorf("configmanager: field %q.%q is not an allowed enum value", field.Section, field.Key)
	case FieldPath:
		if err := validateTextConstraints(field, candidate); err != nil {
			return nil, "", err
		}
		canonical, err := validatePath(*field.Path, candidate)
		if err != nil {
			return nil, "", fmt.Errorf("configmanager: field %q.%q: %w", field.Section, field.Key, err)
		}
		return canonical, canonical, nil
	case FieldString:
		if err := validateTextConstraints(field, candidate); err != nil {
			return nil, "", err
		}
		return candidate, candidate, nil
	default:
		return nil, "", fmt.Errorf("configmanager: unsupported field type %q", field.Type)
	}
}

func validateTextConstraints(field FieldSchema, value string) error {
	length := utf8.RuneCountInString(value)
	if field.MinLength != nil && length < *field.MinLength || field.MaxLength != nil && length > *field.MaxLength {
		return fmt.Errorf("configmanager: field %q.%q has invalid length", field.Section, field.Key)
	}
	if field.Pattern != "" {
		matched, err := regexp.MatchString(field.Pattern, value)
		if err != nil || !matched {
			return fmt.Errorf("configmanager: field %q.%q does not match its required pattern", field.Section, field.Key)
		}
	}
	return nil
}

func validatePath(policy PathPolicy, candidate string) (string, error) {
	if !filepath.IsAbs(candidate) || filepath.Clean(candidate) != candidate {
		return "", fmt.Errorf("path must be absolute and clean")
	}
	canonical := candidate
	info, err := os.Lstat(candidate)
	if err != nil {
		if policy.MustExist || !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect path: %w", err)
		}
		parent, parentErr := filepath.EvalSymlinks(filepath.Dir(candidate))
		if parentErr == nil {
			canonical = filepath.Join(parent, filepath.Base(candidate))
			if canonical != candidate && !policy.AllowSymlink {
				return "", fmt.Errorf("symlinks are not allowed")
			}
		}
	} else {
		resolved, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr != nil {
			return "", fmt.Errorf("resolve path: %w", resolveErr)
		}
		if resolved != candidate && !policy.AllowSymlink {
			return "", fmt.Errorf("symlinks are not allowed")
		}
		canonical = resolved
		info, err = os.Stat(canonical)
		if err != nil {
			return "", fmt.Errorf("inspect resolved path: %w", err)
		}
		switch policy.FileType {
		case "file":
			if !info.Mode().IsRegular() {
				return "", fmt.Errorf("path is not a regular file")
			}
		case "directory":
			if !info.IsDir() {
				return "", fmt.Errorf("path is not a directory")
			}
		}
		if policy.ExpectedMode != "" {
			expected, _ := strconv.ParseUint(policy.ExpectedMode, 8, 12)
			if info.Mode().Perm() != os.FileMode(expected) {
				return "", fmt.Errorf("path mode does not match %s", policy.ExpectedMode)
			}
		}
		if policy.ExpectedUID != nil {
			uid, ok := fileOwnerUID(info)
			if !ok || uid != uint32(*policy.ExpectedUID) {
				return "", fmt.Errorf("path owner does not match uid %d", *policy.ExpectedUID)
			}
		}
	}
	allowed := false
	for _, prefix := range policy.AllowedPrefixes {
		canonicalPrefix := prefix
		if resolved, resolveErr := filepath.EvalSymlinks(prefix); resolveErr == nil {
			canonicalPrefix = resolved
		}
		relative, err := filepath.Rel(canonicalPrefix, canonical)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("path is outside allowed prefixes")
	}
	return canonical, nil
}

func validateCompleteDocument(resource Resource, document document) error {
	values := map[string]any{}
	for _, field := range resource.Fields {
		value, exists := document.Get(field.Section, field.Key)
		if !exists {
			if field.Required {
				return fmt.Errorf("configmanager: required field %q.%q is missing", field.Section, field.Key)
			}
			continue
		}
		normalized, _, err := normalizeValue(field, scalarString(value))
		if err != nil {
			return err
		}
		values[field.Section+"\x00"+field.Key] = normalized
	}
	for _, rule := range resource.CrossFieldRules {
		left, leftOK := values[rule.Section+"\x00"+rule.Key]
		right, rightOK := values[rule.OtherSection+"\x00"+rule.OtherKey]
		if !leftOK || !rightOK {
			continue
		}
		switch rule.Type {
		case "not_equal":
			if scalarString(left) == scalarString(right) {
				return fmt.Errorf("configmanager: cross-field rule requires %s.%s and %s.%s to differ", rule.Section, rule.Key, rule.OtherSection, rule.OtherKey)
			}
		case "less_or_equal":
			leftNumber, leftIsNumber := left.(int64)
			rightNumber, rightIsNumber := right.(int64)
			if !leftIsNumber || !rightIsNumber || leftNumber > rightNumber {
				return fmt.Errorf("configmanager: cross-field rule requires %s.%s <= %s.%s", rule.Section, rule.Key, rule.OtherSection, rule.OtherKey)
			}
		}
	}
	return nil
}

func typedValueForJSON(value any) any {
	if number, ok := value.(int64); ok {
		return json.Number(strconv.FormatInt(number, 10))
	}
	return value
}
