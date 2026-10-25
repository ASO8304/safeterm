// Package resources compiles administrator-owned resource definitions into the
// existing signed catalog and typed Config Manager registry. It never executes
// commands or changes application files.
package resources

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
)

const TemplateName = "TEMPLATE.json"
const MaxFiles = 256
const MaxFileBytes = 1 << 20

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

type Grant struct {
	Roles []string `json:"roles"`
}
type Service struct {
	Unit           string           `json:"unit,omitempty"`
	Units          []string         `json:"units,omitempty"`
	TimeoutSeconds int              `json:"timeout_seconds,omitempty"`
	Actions        map[string]Grant `json:"actions"`
}
type Permissions struct {
	Read     []string `json:"read"`
	Edit     []string `json:"edit,omitempty"`
	Rollback []string `json:"rollback,omitempty"`
}
type ContextField struct {
	Name string `json:"name"`
	configmanager.FieldSchema
}

type ContextDefinition struct {
	Path        []string       `json:"path"`
	Description string         `json:"description,omitempty"`
	Fields      []ContextField `json:"fields"`
}

type Configuration struct {
	Contexts        []ContextDefinition            `json:"contexts,omitempty"`
	Path            string                         `json:"path"`
	Format          configmanager.Format           `json:"format"`
	Profile         string                         `json:"profile,omitempty"`
	BackupKeep      *int                           `json:"backup_keep,omitempty"`
	Permissions     Permissions                    `json:"permissions"`
	Fields          []configmanager.FieldSchema    `json:"fields"`
	CrossFieldRules []configmanager.CrossFieldRule `json:"cross_field_rules,omitempty"`
	OnChange        string                         `json:"on_change,omitempty"`
}
type Log struct {
	ID          string   `json:"id"`
	Path        string   `json:"path"`
	Description string   `json:"description,omitempty"`
	Roles       []string `json:"roles"`
}
type Definition struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Service     *Service       `json:"service,omitempty"`
	Config      *Configuration `json:"config,omitempty"`
	Logs        []Log          `json:"logs,omitempty"`
}
type Named struct {
	File       string
	Definition Definition
}
type Manifest struct {
	Version    int      `json:"version"`
	Operations []string `json:"operations"`
	Resources  []string `json:"resources"`
}

func EmptyManifest() Manifest {
	return Manifest{Version: 1, Operations: []string{}, Resources: []string{}}
}

// Decode rejects unknown/duplicate properties, deep inputs and trailing values.
// The typed decoder runs first so unknown recursive values are never traversed.
func Decode(data []byte, target any) error {
	if len(data) == 0 || len(data) > MaxFileBytes {
		return fmt.Errorf("JSON must be between 1 byte and 1 MiB")
	}
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("JSON root must be an object")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		var syntax *json.SyntaxError
		var mismatch *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntax):
			return jsonPosition(data, syntax.Offset, err)
		case errors.As(err, &mismatch):
			return jsonPosition(data, mismatch.Offset, err)
		case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
			return jsonPosition(data, int64(len(data))+1, err)
		default:
			return err
		}
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	dec = json.NewDecoder(bytes.NewReader(data))
	var value func(int) error
	value = func(depth int) error {
		if depth > 32 {
			return fmt.Errorf("JSON nesting exceeds 32")
		}
		token, err := dec.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delimiter == '{' {
			seen := map[string]bool{}
			for dec.More() {
				token, err = dec.Token()
				if err != nil {
					return err
				}
				key, ok := token.(string)
				if !ok || seen[key] {
					return jsonPosition(data, dec.InputOffset(), fmt.Errorf("duplicate JSON property %q", key))
				}
				seen[key] = true
				if err = value(depth + 1); err != nil {
					return err
				}
			}
		} else if delimiter == '[' {
			for dec.More() {
				if err = value(depth + 1); err != nil {
					return err
				}
			}
		} else {
			return fmt.Errorf("unexpected JSON delimiter")
		}
		_, err = dec.Token()
		return err
	}
	return value(0)
}

// Report a 1-based line/character column without printing application values.
// Offset follows encoding/json's 1-based byte position convention.
func jsonPosition(data []byte, offset int64, err error) error {
	index := int(offset - 1)
	if index < 0 {
		index = 0
	}
	if index > len(data) {
		index = len(data)
	}
	prefix := data[:index]
	line := bytes.Count(prefix, []byte("\n")) + 1
	start := bytes.LastIndexByte(prefix, '\n') + 1
	column := utf8.RuneCount(prefix[start:]) + 1
	return fmt.Errorf("line %d, column %d (byte %d): %w", line, column, index+1, err)
}

func Parse(file string, data []byte) (Named, error) {
	var d Definition
	if err := Decode(data, &d); err != nil {
		return Named{}, fmt.Errorf("%s: %w", file, err)
	}
	if !idPattern.MatchString(d.ID) {
		return Named{}, fmt.Errorf("%s: id must match %s", file, idPattern)
	}
	if d.Service == nil && d.Config == nil && len(d.Logs) == 0 {
		return Named{}, fmt.Errorf("%s: at least one of service, config or logs is required", file)
	}
	if strings.TrimSpace(d.Description) == "" || len(d.Description) > 1024 {
		return Named{}, fmt.Errorf("%s: description is required (max 1024 bytes)", file)
	}
	return Named{file, d}, nil
}

func OperationID(id, kind, name string) string { return "resource_" + id + "_" + kind + "_" + name }
func OperationWords(name string) ([]string, bool) {
	if !strings.HasPrefix(name, "resource_") {
		return nil, false
	}
	rest := strings.TrimPrefix(name, "resource_")
	for _, kind := range []string{"service", "log"} {
		parts := strings.Split(rest, "_"+kind+"_")
		if len(parts) != 2 || !idPattern.MatchString(parts[0]) || !idPattern.MatchString(parts[1]) {
			continue
		}
		if kind == "service" {
			return []string{"service", parts[1], parts[0]}, true
		}
		return []string{"logs", parts[0], parts[1]}, true
	}
	return nil, false
}

func ValidateLogPath(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("log path must be canonical and absolute")
	}
	for _, c := range path {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("/_-.", c)) {
			return fmt.Errorf("unsupported log path character")
		}
	}
	// These namespaces are hidden/private in the log reader's service sandbox.
	for _, p := range []string{"/proc", "/sys", "/dev", "/run", "/tmp", "/var/tmp", "/home", "/root", "/etc/safeterm", "/var/lib/safeterm-installer"} {
		if path == p || strings.HasPrefix(path, p+"/") {
			return fmt.Errorf("log path is in a protected or sandbox-hidden namespace")
		}
	}
	return nil
}

// Compile derives one complete generation from the trusted packaged built-ins
// and resources.d. Previous active catalogs/registries are never compiler input.
func Compile(base *config.Config, items []Named) (*config.Config, *configmanager.Registry, Manifest, error) {
	if len(items) > MaxFiles {
		return nil, nil, Manifest{}, fmt.Errorf("excessive definitions")
	}
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, nil, Manifest{}, err
	}
	var cfg config.Config
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return nil, nil, Manifest{}, err
	}
	reg := configmanager.Registry{Version: "resources-v2", Resources: []configmanager.Resource{}}
	manifest := EmptyManifest()
	knownRoles := map[string]bool{}
	knownOps := map[string]bool{}
	knownResources := map[string]bool{}
	for _, r := range cfg.Roles {
		knownRoles[r.Name] = true
	}
	for _, o := range cfg.Operations {
		knownOps[o.Name] = true
	}
	for _, r := range reg.Resources {
		knownResources[r.ID] = true
	}
	add := func(op config.OperationDefinition, roles []string) error {
		if knownOps[op.Name] {
			return fmt.Errorf("operation %s conflicts with an existing definition", op.Name)
		}
		if len(roles) == 0 {
			return fmt.Errorf("%s.roles must not be empty", op.Name)
		}
		seen := map[string]bool{}
		for _, role := range roles {
			if !knownRoles[role] || seen[role] {
				return fmt.Errorf("%s: unknown or duplicate role %s", op.Name, role)
			}
			seen[role] = true
			found := false
			for i := range cfg.Policies {
				if cfg.Policies[i].Role == role {
					cfg.Policies[i].Operations = append(cfg.Policies[i].Operations, op.Name)
					found = true
					break
				}
			}
			if !found {
				cfg.Policies = append(cfg.Policies, config.PolicyDefinition{Role: role, Operations: []string{op.Name}})
			}
		}
		knownOps[op.Name] = true
		cfg.Operations = append(cfg.Operations, op)
		manifest.Operations = append(manifest.Operations, op.Name)
		return nil
	}
	seen := map[string]bool{}
	sort.Slice(items, func(i, j int) bool { return items[i].File < items[j].File })
	for _, item := range items {
		d := item.Definition
		fail := func(e error) (*config.Config, *configmanager.Registry, Manifest, error) {
			return nil, nil, Manifest{}, fmt.Errorf("%s: %w", item.File, e)
		}
		if !idPattern.MatchString(d.ID) || seen[d.ID] {
			return fail(fmt.Errorf("invalid or duplicate id %s", d.ID))
		}
		seen[d.ID] = true
		if d.Service != nil {
			if err := config.ValidateSystemdDefinition(&config.SystemdOperationDefinition{Unit: d.Service.Unit, Units: d.Service.Units, TimeoutSeconds: d.Service.TimeoutSeconds, Action: "status"}); err != nil {
				return fail(fmt.Errorf("service: %w", err))
			}
			if len(d.Service.Actions) == 0 {
				return fail(fmt.Errorf("service.actions must not be empty"))
			}
			actions := []string{}
			for action := range d.Service.Actions {
				actions = append(actions, action)
			}
			sort.Strings(actions)
			for _, action := range actions {
				switch action {
				case "status", "start", "stop", "restart", "reload", "enable", "disable":
				default:
					return fail(fmt.Errorf("unsupported service action %s", action))
				}
				op := config.OperationDefinition{Name: OperationID(d.ID, "service", action), Description: d.Description + ": " + action, RunnerProfile: config.ProfileServiceManager, ExecutorKind: config.ExecutorSystemd, Systemd: &config.SystemdOperationDefinition{Unit: d.Service.Unit, Units: append([]string(nil), d.Service.Units...), Alias: d.ID, TimeoutSeconds: d.Service.TimeoutSeconds, Action: action}, Parameters: []config.ParameterDefinition{}}
				if err := add(op, d.Service.Actions[action].Roles); err != nil {
					return fail(err)
				}
			}
		}
		if c := d.Config; c != nil {
			if knownResources[d.ID] {
				return fail(fmt.Errorf("config id %s conflicts with an existing registry entry; choose another id or migrate explicitly", d.ID))
			}
			keep := 5
			if c.BackupKeep != nil {
				keep = *c.BackupKeep
			}
			if keep < 1 || keep > 32 {
				return fail(fmt.Errorf("config.backup_keep must be 1..32"))
			}
			profile := c.Profile
			if profile == "" {
				profile = "application"
			}
			if profile != "application" && profile != "database" && profile != "network" {
				return fail(fmt.Errorf("invalid config.profile"))
			}
			access := configmanager.AccessReadOnly
			if len(c.Permissions.Edit) > 0 {
				access = configmanager.AccessReadWrite
			}
			r := configmanager.Resource{ID: d.ID, Description: d.Description, Profile: profile, Path: c.Path, Format: c.Format, Access: access, PrivilegedWrite: true, Fields: c.Fields, CrossFieldRules: c.CrossFieldRules, Backup: configmanager.BackupPolicy{Enabled: true, Keep: keep}, Permissions: configmanager.Permissions{ReadRoles: c.Permissions.Read, ProposeRoles: c.Permissions.Edit, ApplyRoles: c.Permissions.Edit, RollbackRoles: c.Permissions.Rollback}}

			if c.OnChange != "" {
				return fail(fmt.Errorf("config.on_change is no longer supported; save the file and use execute service separately"))
			}
			if len(c.Contexts) > 0 {
				if len(c.Contexts) > 128 {
					return fail(fmt.Errorf("too many configuration contexts"))
				}
				if len(c.Fields) > 0 {
					return fail(fmt.Errorf("use config.contexts or config.fields, not both"))
				}
				for _, context := range c.Contexts {
					if len(context.Fields) == 0 || len(context.Fields) > configmanager.MaxPatchChanges {
						return fail(fmt.Errorf("context fields must contain 1..512 entries"))
					}
					compiled := configmanager.ConfigContext{Path: append([]string(nil), context.Path...), Description: context.Description}
					for _, field := range context.Fields {
						r.Fields = append(r.Fields, field.FieldSchema)
						compiled.Fields = append(compiled.Fields, configmanager.FieldAlias{Name: field.Name, Section: field.Section, Key: field.Key})
					}
					r.Contexts = append(r.Contexts, compiled)
				}
			}

			one := configmanager.Registry{Version: reg.Version, Resources: []configmanager.Resource{r}}
			if err := configmanager.ValidateRegistry(&one); err != nil {
				return fail(err)
			}
			r = one.Resources[0]
			for _, roles := range [][]string{r.Permissions.ReadRoles, r.Permissions.ProposeRoles, r.Permissions.RollbackRoles, r.Permissions.ActivateRoles} {
				for _, role := range roles {
					if !knownRoles[role] {
						return fail(fmt.Errorf("config.permissions: unknown role %s", role))
					}
				}
			}
			reg.Resources = append(reg.Resources, r)
			knownResources[d.ID] = true
			manifest.Resources = append(manifest.Resources, d.ID)
		}
		seenLogs := map[string]bool{}
		for _, log := range d.Logs {
			if !idPattern.MatchString(log.ID) || seenLogs[log.ID] {
				return fail(fmt.Errorf("invalid or duplicate logs.id"))
			}
			seenLogs[log.ID] = true
			if err := ValidateLogPath(log.Path); err != nil {
				return fail(fmt.Errorf("logs.%s.path: %w", log.ID, err))
			}
			desc := log.Description
			if desc == "" {
				desc = d.Description + ": " + log.ID
			}
			op := config.OperationDefinition{Name: OperationID(d.ID, "log", log.ID), Description: desc, RunnerProfile: config.ProfileLogReader, ExecutorKind: config.ExecutorProcess, ExecutableRef: "tail", ArgsTemplate: []string{"-n", "50", "-F", "--", log.Path}, Parameters: []config.ParameterDefinition{}, LongRunning: true}
			if err := add(op, log.Roles); err != nil {
				return fail(err)
			}
		}
	}
	sort.Slice(cfg.Operations, func(i, j int) bool { return cfg.Operations[i].Name < cfg.Operations[j].Name })
	sort.Slice(cfg.Policies, func(i, j int) bool { return cfg.Policies[i].Role < cfg.Policies[j].Role })
	for i := range cfg.Policies {
		sort.Strings(cfg.Policies[i].Operations)
	}
	sort.Slice(reg.Resources, func(i, j int) bool { return reg.Resources[i].ID < reg.Resources[j].ID })
	sort.Strings(manifest.Operations)
	sort.Strings(manifest.Resources)
	if err := config.ValidateConfig(&cfg); err != nil {
		return nil, nil, Manifest{}, err
	}
	if err := configmanager.ValidateRegistry(&reg); err != nil {
		return nil, nil, Manifest{}, err
	}
	for _, r := range reg.Resources {
		for _, roles := range [][]string{r.Permissions.ReadRoles, r.Permissions.ProposeRoles, r.Permissions.RollbackRoles, r.Permissions.ActivateRoles} {
			for _, role := range roles {
				if !knownRoles[role] {
					return nil, nil, Manifest{}, fmt.Errorf("resource %s: unknown role %s", r.ID, role)
				}
			}
		}
	}
	return &cfg, &reg, manifest, nil
}
