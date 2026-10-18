package broker

import (
	"fmt"
	"sort"

	"safeterm/internal/config"
)

// Catalog is the Broker's immutable, presentation-only view of the signed
// operation catalog and RBAC policy. It cannot translate or execute commands.
type Catalog struct {
	operations map[string]config.OperationDefinition
	allowed    map[string]map[string]bool
	roles      map[string]bool
}

func NewCatalog(cfg *config.Config) (*Catalog, error) {
	if err := config.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("broker: invalid catalog: %w", err)
	}
	catalog := &Catalog{
		operations: make(map[string]config.OperationDefinition, len(cfg.Operations)),
		allowed:    make(map[string]map[string]bool, len(cfg.Policies)),
		roles:      make(map[string]bool, len(cfg.Roles)),
	}
	for _, role := range cfg.Roles {
		catalog.roles[role.Name] = true
	}
	for _, operation := range cfg.Operations {
		clone := operation
		clone.ArgsTemplate = append([]string(nil), operation.ArgsTemplate...)
		clone.RequiredAny = append([]string(nil), operation.RequiredAny...)
		clone.ArgumentGroupSeparator = append([]string(nil), operation.ArgumentGroupSeparator...)
		clone.ArgumentGroups = append([]config.ArgumentGroup(nil), operation.ArgumentGroups...)
		for i := range clone.ArgumentGroups {
			clone.ArgumentGroups[i].ArgsTemplate = append([]string(nil), operation.ArgumentGroups[i].ArgsTemplate...)
		}
		clone.Parameters = append([]config.ParameterDefinition(nil), operation.Parameters...)
		catalog.operations[operation.Name] = clone
	}
	for _, policy := range cfg.Policies {
		if catalog.allowed[policy.Role] == nil {
			catalog.allowed[policy.Role] = map[string]bool{}
		}
		for _, operation := range policy.Operations {
			catalog.allowed[policy.Role][operation] = true
		}
	}
	return catalog, nil
}

func (c *Catalog) KnownRoles() map[string]bool {
	result := map[string]bool{}
	if c != nil {
		for role := range c.roles {
			result[role] = true
		}
	}
	return result
}

func (c *Catalog) AuthorizedOperations(roles []string) []config.OperationDefinition {
	if c == nil {
		return nil
	}
	visible := map[string]bool{}
	for _, role := range roles {
		for operation := range c.allowed[role] {
			visible[operation] = true
		}
	}
	names := make([]string, 0, len(visible))
	for name := range visible {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]config.OperationDefinition, 0, len(names))
	for _, name := range names {
		result = append(result, c.operations[name])
	}
	return result
}

func (c *Catalog) AuthorizedOperation(name string, roles []string) (config.OperationDefinition, bool) {
	if c == nil {
		return config.OperationDefinition{}, false
	}
	operation, exists := c.operations[name]
	if !exists {
		return config.OperationDefinition{}, false
	}
	for _, role := range roles {
		if c.allowed[role][name] {
			return operation, true
		}
	}
	return config.OperationDefinition{}, false
}
