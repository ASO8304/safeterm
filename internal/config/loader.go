package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadConfig reads every *.json file inside <baseDir>/operations,
// <baseDir>/roles and <baseDir>/policies, merges the results, and runs
// structural validation (ValidateConfig) before returning.
//
// It never returns a partially-valid Config: either everything parses and
// is internally consistent, or an error is returned and the caller must
// not start the Agent with it. Supporting multiple files per folder is
// deliberate — adding a new operation should mean "drop a new JSON file
// into configs/operations/", not "edit one giant file" (see REQ-003).
func LoadConfig(baseDir string) (*Config, error) {
	operations, err := loadJSONArrayFiles[OperationDefinition](filepath.Join(baseDir, "operations"))
	if err != nil {
		return nil, fmt.Errorf("config: loading operations: %w", err)
	}

	roles, err := loadJSONArrayFiles[RoleDefinition](filepath.Join(baseDir, "roles"))
	if err != nil {
		return nil, fmt.Errorf("config: loading roles: %w", err)
	}

	policies, err := loadJSONArrayFiles[PolicyDefinition](filepath.Join(baseDir, "policies"))
	if err != nil {
		return nil, fmt.Errorf("config: loading policies: %w", err)
	}

	cfg := &Config{
		Operations: operations,
		Roles:      roles,
		Policies:   policies,
	}

	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config: validation failed: %w", err)
	}

	return cfg, nil
}

// loadJSONArrayFiles reads every *.json file in dir, unmarshals each one as
// a JSON array of T, and concatenates the results. Non-.json files
// (like .gitkeep) are silently skipped.
func loadJSONArrayFiles[T any](dir string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var all []T
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}

		var items []T
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		all = append(all, items...)
	}

	return all, nil
}
