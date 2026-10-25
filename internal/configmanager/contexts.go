package configmanager

import (
	"fmt"
	"regexp"
	"strings"
)

var contextTokenPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

func validateContexts(resource Resource) error {
	if len(resource.Contexts) > 128 {
		return fmt.Errorf("configmanager: resource %q has too many contexts", resource.ID)
	}
	if len(resource.Contexts) > 0 && (resource.ActivationOperation != "" || resource.HealthCheckOperation != "") {
		return fmt.Errorf("configmanager: logical contexts save files only; use execute service for service actions")
	}
	for _, c := range resource.Contexts {
		if len(c.Path) == 0 || len(c.Path) > 8 || len(c.Description) > 1024 || len(c.Fields) == 0 || len(c.Fields) > MaxPatchChanges {
			return fmt.Errorf("configmanager: resource %q has invalid context shape", resource.ID)
		}
		for _, token := range c.Path {
			if !contextTokenPattern.MatchString(token) {
				return fmt.Errorf("configmanager: invalid context path token %q", token)
			}
		}
		names := map[string]bool{}
		coordinates := map[[2]string]bool{}
		for _, alias := range c.Fields {
			coordinate := [2]string{alias.Section, alias.Key}
			if !fieldIdentifierPattern.MatchString(alias.Name) || names[alias.Name] || coordinates[coordinate] {
				return fmt.Errorf("configmanager: resource %q context %q has invalid or duplicate field alias %q", resource.ID, strings.Join(c.Path, " "), alias.Name)
			}
			if _, ok := findField(resource, alias.Section, alias.Key); !ok {
				return fmt.Errorf("configmanager: alias %q references an unregistered field", alias.Name)
			}
			names[alias.Name] = true
			coordinates[coordinate] = true
		}
	}
	return nil
}

type contextRouteNode struct {
	leaf     bool
	children map[string]*contextRouteNode
}

// ValidateContextRoutes rejects ambiguous paths, including legacy ID fallbacks.
// A trie bounds work to total route length instead of comparing every pair.
func ValidateContextRoutes(items []ResourceSummary) error {
	root := &contextRouteNode{}
	total := 0
	for _, item := range items {
		paths := [][]string{}
		if len(item.Contexts) == 0 {
			paths = append(paths, []string{item.ID})
		} else {
			for _, c := range item.Contexts {
				paths = append(paths, c.Path)
			}
		}
		for _, path := range paths {
			total++
			if total > 4096 || len(path) == 0 || len(path) > 8 {
				return fmt.Errorf("configmanager: excessive or invalid config routes")
			}
			node := root
			for _, token := range path {
				if node.leaf {
					return fmt.Errorf("configmanager: conflicting configuration path %q", strings.Join(path, " "))
				}
				if node.children == nil {
					node.children = map[string]*contextRouteNode{}
				}
				if node.children[token] == nil {
					node.children[token] = &contextRouteNode{}
				}
				node = node.children[token]
			}
			if node.leaf || len(node.children) > 0 {
				return fmt.Errorf("configmanager: conflicting configuration path %q", strings.Join(path, " "))
			}
			node.leaf = true
		}
	}
	return nil
}

func cloneContexts(items []ConfigContext) []ConfigContext {
	if items == nil {
		return nil
	}
	result := append([]ConfigContext(nil), items...)
	for i := range result {
		result[i].Path = append([]string(nil), items[i].Path...)
		result[i].Fields = append([]FieldAlias(nil), items[i].Fields...)
	}
	return result
}
