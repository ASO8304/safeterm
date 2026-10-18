// Package siteconfig compiles administrator JSON overlays over verified defaults.
package siteconfig

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"safeterm/internal/config"
	"safeterm/internal/resources"
)

var Directories = []string{"roles.d", "profiles.d", "executables.d", "operations.d", "policies.d"}

const MaxProfiles = 16

var SiteName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,19}$`)

type Named[T any] struct {
	File  string
	Value T
}
type Role struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
type Operation struct {
	Operation config.OperationDefinition `json:"operation"`
	Roles     *[]string                  `json:"roles,omitempty"`
	Disabled  bool                       `json:"disabled,omitempty"`
}
type Profile struct {
	Name               string               `json:"name"`
	Base               config.RunnerProfile `json:"base"`
	MemoryLimitMiB     int                  `json:"memory_limit_mib,omitempty"`
	ProcessLimit       int                  `json:"process_limit,omitempty"`
	Capabilities       *[]string            `json:"capabilities,omitempty"`
	TimeoutSeconds     int                  `json:"timeout_seconds,omitempty"`
	OutputLimitBytes   int                  `json:"output_limit_bytes,omitempty"`
	MaxSessions        int                  `json:"max_sessions,omitempty"`
	IdleTimeoutSeconds int                  `json:"idle_timeout_seconds,omitempty"`
}
type Inputs struct {
	Roles       []Named[Role]
	Profiles    []Named[Profile]
	Executables []Named[config.ExecutableDefinition]
	Operations  []Named[Operation]
	Policies    []Named[config.PolicyDefinition]
}

func Parse[T any](file string, data []byte) (Named[T], error) {
	var value T
	if err := resources.Decode(data, &value); err != nil {
		return Named[T]{}, fmt.Errorf("%s: %w", file, err)
	}
	return Named[T]{File: file, Value: value}, nil
}

func Clone(base *config.Config) (*config.Config, error) {
	data, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var result config.Config
	err = json.Unmarshal(data, &result)
	return &result, err
}

// Prepare must run before resources.Compile so resource permissions can use custom roles.
func Prepare(base *config.Config, input Inputs) (*config.Config, error) {
	if base == nil {
		return nil, fmt.Errorf("missing verified defaults")
	}
	cfg, err := Clone(base)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, n := range input.Roles {
		v := n.Value
		if !validName(v.Name) || seen[v.Name] || strings.TrimSpace(v.Description) == "" {
			return nil, fmt.Errorf("%s: invalid or duplicate role", n.File)
		}
		seen[v.Name] = true
		found := false
		for i := range cfg.Roles {
			if cfg.Roles[i].Name == v.Name {
				cfg.Roles[i] = config.RoleDefinition{Name: v.Name, Description: v.Description}
				found = true
			}
		}
		if !found {
			cfg.Roles = append(cfg.Roles, config.RoleDefinition{Name: v.Name, Description: v.Description})
		}
	}
	seen = map[string]bool{}
	if len(input.Profiles) > MaxProfiles+4 {
		return nil, fmt.Errorf("too many profile definitions")
	}
	for _, n := range input.Profiles {
		v := n.Value
		if seen[v.Name] {
			return nil, fmt.Errorf("%s: duplicate profile", n.File)
		}
		seen[v.Name] = true
		var b *config.ProfileDefinition
		for i := range base.Profiles {
			if base.Profiles[i].Name == v.Base {
				b = &base.Profiles[i]
			}
		}
		if b == nil || !Builtin(v.Base) {
			return nil, fmt.Errorf("%s: base must name a packaged isolation template", n.File)
		}
		p := *b
		found := -1
		for i := range cfg.Profiles {
			if string(cfg.Profiles[i].Name) == v.Name {
				found = i
			}
		}
		if found >= 0 {
			if config.RunnerProfile(v.Name) != v.Base {
				return nil, fmt.Errorf("%s: built-in profile cannot change isolation template", n.File)
			}
		} else {
			if !SiteName.MatchString(v.Name) || Builtin(config.RunnerProfile(v.Name)) {
				return nil, fmt.Errorf("%s: invalid site profile name", n.File)
			}
			p.Name = config.RunnerProfile(v.Name)
			p.RunAsUser = User(v.Name)
			p.SocketPath = Socket(v.Name)
			p.Template = v.Base
		}
		if v.MemoryLimitMiB != 0 {
			p.MemoryLimitMiB = v.MemoryLimitMiB
		}
		if v.ProcessLimit != 0 {
			p.ProcessLimit = v.ProcessLimit
		}
		if v.Capabilities != nil {
			p.Capabilities = append([]string{}, (*v.Capabilities)...)
		}
		p.TimeoutSeconds = v.TimeoutSeconds
		p.OutputLimitBytes = v.OutputLimitBytes
		p.MaxSessions = v.MaxSessions
		p.IdleTimeoutSeconds = v.IdleTimeoutSeconds
		if err := ValidateProfile(p); err != nil {
			return nil, fmt.Errorf("%s: %w", n.File, err)
		}
		if found >= 0 {
			cfg.Profiles[found] = p
		} else {
			cfg.Profiles = append(cfg.Profiles, p)
		}
		// Template executables remain individually hashed, with explicit profile membership.
		for i := range cfg.Executables {
			e := &cfg.Executables[i]
			if contains(e.AllowedProfiles, v.Base) && !contains(e.AllowedProfiles, p.Name) {
				e.AllowedProfiles = append(e.AllowedProfiles, p.Name)
			}
		}
	}
	custom := 0
	for _, p := range cfg.Profiles {
		if p.Template != "" {
			custom++
		}
	}
	if custom > MaxProfiles {
		return nil, fmt.Errorf("at most %d site profiles", MaxProfiles)
	}
	seen = map[string]bool{}
	for _, n := range input.Executables {
		e := n.Value
		if seen[e.ID] {
			return nil, fmt.Errorf("%s: duplicate executable", n.File)
		}
		seen[e.ID] = true
		found := false
		for i := range cfg.Executables {
			if cfg.Executables[i].ID == e.ID {
				cfg.Executables[i] = e
				found = true
			}
		}
		if !found {
			cfg.Executables = append(cfg.Executables, e)
		}
	}
	seen = map[string]bool{}
	for _, n := range input.Operations {
		o := n.Value.Operation
		if !validName(o.Name) || seen[o.Name] || strings.HasPrefix(o.Name, "resource_") {
			return nil, fmt.Errorf("%s: invalid, reserved or duplicate operation", n.File)
		}
		seen[o.Name] = true
		found := false
		for i := 0; i < len(cfg.Operations); i++ {
			if cfg.Operations[i].Name == o.Name {
				found = true
				if n.Value.Disabled {
					cfg.Operations = append(cfg.Operations[:i], cfg.Operations[i+1:]...)
				} else {
					cfg.Operations[i] = o
				}
				break
			}
		}
		if n.Value.Disabled {
			for i := range cfg.Policies {
				cfg.Policies[i].Operations = without(cfg.Policies[i].Operations, o.Name)
			}
			continue
		}
		if !found {
			cfg.Operations = append(cfg.Operations, o)
		}
		if n.Value.Roles != nil {
			seenRoles := map[string]bool{}
			for _, role := range *n.Value.Roles {
				if seenRoles[role] {
					return nil, fmt.Errorf("%s: duplicate operation role %s", n.File, role)
				}
				seenRoles[role] = true
			}
			for i := range cfg.Policies {
				cfg.Policies[i].Operations = without(cfg.Policies[i].Operations, o.Name)
			}
			for _, role := range *n.Value.Roles {
				grant(cfg, role, o.Name)
			}
		}
	}
	return cfg, nil
}

// Finish applies exact policy replacements after generated resource operations exist.
func Finish(cfg *config.Config, input Inputs) error {
	seen := map[string]bool{}
	for _, n := range input.Policies {
		p := n.Value
		seenOps := map[string]bool{}
		for _, op := range p.Operations {
			if seenOps[op] {
				return fmt.Errorf("%s: duplicate operation %s in policy", n.File, op)
			}
			seenOps[op] = true
		}
		if seen[p.Role] {
			return fmt.Errorf("%s: duplicate role policy", n.File)
		}
		seen[p.Role] = true
		found := false
		for i := range cfg.Policies {
			if cfg.Policies[i].Role == p.Role {
				cfg.Policies[i] = p
				found = true
			}
		}
		if !found {
			cfg.Policies = append(cfg.Policies, p)
		}
	}
	for i := range cfg.Policies {
		sort.Strings(cfg.Policies[i].Operations)
	}
	return config.ValidateConfig(cfg)
}
func validName(s string) bool { return regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`).MatchString(s) }
func contains(v []config.RunnerProfile, p config.RunnerProfile) bool {
	for _, n := range v {
		if n == p {
			return true
		}
	}
	return false
}
func without(v []string, s string) []string {
	result := []string{}
	for _, n := range v {
		if n != s {
			result = append(result, n)
		}
	}
	return result
}
func grant(c *config.Config, role, op string) {
	for i := range c.Policies {
		if c.Policies[i].Role == role {
			c.Policies[i].Operations = append(c.Policies[i].Operations, op)
			return
		}
	}
	c.Policies = append(c.Policies, config.PolicyDefinition{Role: role, Operations: []string{op}})
}
