//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"safeterm/internal/config"
	"safeterm/internal/resources"
	"safeterm/internal/siteconfig"
)

func loadSiteDirectory[T any](root, dir string) ([]siteconfig.Named[T], error) {
	path := filepath.Join(root, dir)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if err := catalogSecureDirectory(path); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	result := []siteconfig.Named[T]{}
	total := 0
	for _, e := range entries {
		if e.Name() == resources.TemplateName || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		if len(result) >= resources.MaxFiles {
			return nil, fmt.Errorf("%s: too many JSON inputs", dir)
		}
		data, err := resourceReadRoot(filepath.Join(path, e.Name()))
		if err != nil {
			return nil, err
		}
		total += len(data)
		if total > 8<<20 {
			return nil, fmt.Errorf("%s: inputs exceed 8 MiB", dir)
		}
		item, err := siteconfig.Parse[T](filepath.Join(dir, e.Name()), data)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
func loadSiteInputs(root string) (siteconfig.Inputs, error) {
	var input siteconfig.Inputs
	var err error
	if input.Roles, err = loadSiteDirectory[siteconfig.Role](root, "access/roles.d"); err != nil {
		return input, err
	}
	if input.Profiles, err = loadSiteDirectory[siteconfig.Profile](root, "commands/profiles.d"); err != nil {
		return input, err
	}
	if input.Executables, err = loadSiteDirectory[config.ExecutableDefinition](root, "commands/executables.d"); err != nil {
		return input, err
	}
	if input.Operations, err = loadSiteDirectory[siteconfig.Operation](root, "commands/operations.d"); err != nil {
		return input, err
	}
	input.Policies, err = loadSiteDirectory[config.PolicyDefinition](root, "access/policies.d")
	return input, err
}
