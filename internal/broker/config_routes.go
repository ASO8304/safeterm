package broker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"safeterm/internal/configmanager"
)

type configRoute struct {
	path        []string
	resource    string
	description string
	view        *configmanager.ConfigContext
}

func (s *Shell) configRoutes(ctx context.Context) ([]configRoute, error) {
	if s.configs == nil {
		return nil, fmt.Errorf("configuration management is unavailable")
	}
	items, err := s.configs.List(ctx, s.principal)
	if err != nil {
		return nil, err
	}
	if err := configmanager.ValidateContextRoutes(items); err != nil {
		return nil, err
	}
	routes := []configRoute{}
	for _, item := range items {
		if len(item.Contexts) == 0 {
			routes = append(routes, configRoute{path: []string{item.ID}, resource: item.ID, description: item.Description})
			continue
		}
		for _, c := range item.Contexts {
			desc := c.Description
			if desc == "" {
				desc = item.Description
			}
			routes = append(routes, configRoute{path: c.Path, resource: item.ID, description: desc, view: &c})
		}
	}
	sort.Slice(routes, func(i, j int) bool { return strings.Join(routes[i].path, " ") < strings.Join(routes[j].path, " ") })
	return routes, nil
}

func (s *Shell) configPath(ctx context.Context, path []string, help bool) error {
	routes, err := s.configRoutes(ctx)
	if s.network != nil && len(path) == 0 {
		s.writeTerminalData([]byte("network                  Interfaces and routes; use config network ?\n"))
	}
	if err != nil {
		return err
	}
	for _, r := range routes {
		if len(path) != len(r.path) || !wordsStart(r.path, path) {
			continue
		}
		if help {
			desc, err := s.configs.Describe(ctx, s.principal, r.resource)
			if err != nil {
				return err
			}
			fields := desc.Fields
			if r.view != nil {
				fields = aliasDescriptions(desc.Fields, *r.view)
			}
			for _, f := range fields {
				s.writeTerminalData([]byte(fmt.Sprintf("%-24s %s %s\n", f.Key, f.Type, f.Description)))
			}
			return nil
		}
		if err := s.enterConfig(ctx, r.resource); err != nil {
			return err
		}
		if r.view != nil {
			e := s.editing
			e.logical = true
			e.path = append([]string(nil), r.path...)
			e.inSection = true
			e.coordinates = map[string]configmanager.FieldAlias{}
			for _, alias := range r.view.Fields {
				e.coordinates[alias.Name] = alias
			}
			e.description.Fields = aliasDescriptions(e.description.Fields, *r.view)
			e.base = e.aliasRead(e.base)
			fmt.Fprint(s.output, "Use set NAME VALUE, show, diff; end saves only the file; abort discards.\r\n")
		} else {
			fmt.Fprint(s.output, "Use edit SECTION, show, ?; end saves, cancel discards.\r\n")
		}
		return nil
	}
	found := false
	seen := map[string]bool{}
	for _, r := range routes {
		if len(r.path) > len(path) && wordsStart(r.path, path) {
			name := r.path[len(path)]
			if seen[name] {
				continue
			}
			seen[name] = true
			found = true
			s.writeTerminalData([]byte(fmt.Sprintf("%-24s %s\n", name, r.description)))
		}
	}
	if !found && len(path) > 0 {
		return fmt.Errorf("configuration path is unknown or unauthorized; use config ?")
	}
	return nil
}

func aliasDescriptions(fields []configmanager.FieldDescription, c configmanager.ConfigContext) []configmanager.FieldDescription {
	result := []configmanager.FieldDescription{}
	for _, alias := range c.Fields {
		for _, f := range fields {
			if f.Section == alias.Section && f.Key == alias.Key {
				f.Section = ""
				f.Key = alias.Name
				result = append(result, f)
				break
			}
		}
	}
	return result
}

func (e *configEditSession) aliasRead(read configmanager.ReadResult) configmanager.ReadResult {
	if !e.logical {
		return read
	}
	original := read.Fields
	read.Fields = nil
	for name, alias := range e.coordinates {
		for _, f := range original {
			if f.Section == alias.Section && f.Key == alias.Key {
				f.Section = ""
				f.Key = name
				read.Fields = append(read.Fields, f)
				break
			}
		}
	}
	sort.Slice(read.Fields, func(i, j int) bool { return read.Fields[i].Key < read.Fields[j].Key })
	return read
}

func (s *Shell) configCompletion(ctx context.Context, before, prefix string) []string {
	fields := strings.Fields(before)
	parents := fields[1:]
	if !strings.HasSuffix(before, " ") && len(parents) > 0 {
		parents = parents[:len(parents)-1]
	}
	routes, err := s.configRoutes(ctx)
	if err != nil {
		return nil
	}
	values := []string{}
	for _, r := range routes {
		if len(r.path) > len(parents) && wordsStart(r.path, parents) {
			values = append(values, r.path[len(parents)])
		}
	}
	return matching(values, prefix)
}
