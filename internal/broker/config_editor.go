package broker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"safeterm/internal/configmanager"
)

type configEditSession struct {
	resource    string
	section     string
	inSection   bool
	base        configmanager.ReadResult
	description configmanager.ResourceDescription
	changes     []configmanager.FieldChange
}

func (s *Shell) enterConfig(ctx context.Context, id string) error {
	if s.configs == nil {
		return fmt.Errorf("configuration management is unavailable")
	}
	desc, err := s.configs.Describe(ctx, s.principal, id)
	if err != nil {
		return err
	}
	read, err := s.configs.Read(ctx, s.principal, id)
	if err != nil {
		return err
	}
	s.editing = &configEditSession{resource: id, base: read, description: desc}
	fmt.Fprint(s.output, "Use edit SECTION, show, ?; end saves, cancel discards.\r\n")
	return nil
}

func (s *Shell) editPatch() configmanager.Patch {
	e := s.editing
	return configmanager.Patch{ResourceID: e.resource, BaseVersion: e.base.Version, Changes: append([]configmanager.FieldChange(nil), e.changes...)}
}

func (s *Shell) configEditor(ctx context.Context, line string, input <-chan byte) (bool, error) {
	e := s.editing
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false, nil
	}
	if fields[len(fields)-1] == "?" {
		return false, s.editorHelp(fields[:len(fields)-1])
	}
	command := fields[0]
	switch command {
	case "cancel":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: cancel")
		}
		s.editing = nil
		fmt.Fprint(s.output, "Unsaved changes discarded.\r\n")
	case "exit", "logout", "quit":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: exit")
		}
		s.editing = nil
		return true, nil
	case "help":
		return false, s.editorHelp(nil)
	case "edit":
		if len(fields) != 2 {
			return false, fmt.Errorf("usage: edit SECTION (use - for root fields)")
		}
		section := fields[1]
		if section == "-" {
			section = ""
		}
		for _, f := range e.description.Fields {
			if f.Section == section {
				e.section = section
				e.inSection = true
				return false, nil
			}
		}
		return false, fmt.Errorf("section is unknown or unauthorized; use edit ?")
	case "next":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: next")
		}
		// Keep one resource transaction across sections; only end writes the file.
		e.section = ""
		e.inSection = false
	case "show":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: show")
		}
		for _, f := range e.base.Fields {
			if e.inSection && f.Section != e.section {
				continue
			}
			value := f.Value
			for _, c := range e.changes {
				if c.Section == f.Section && c.Key == f.Key {
					value = c.Value
					if f.Redacted {
						value = "[REDACTED]"
					}
				}
			}
			name := f.Key
			if !e.inSection && f.Section != "" {
				name = f.Section + "." + f.Key
			}
			s.writeTerminalData([]byte(fmt.Sprintf("%-32s = %s\n", name, value)))
		}
	case "set":
		if !e.inSection {
			return false, fmt.Errorf("select a section with edit SECTION first")
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "set"))
		split := strings.IndexAny(rest, " \t")
		parts := []string{rest}
		if split >= 0 {
			parts = []string{rest[:split], strings.TrimSpace(rest[split:])}
		}
		key := parts[0]
		var schema *configmanager.FieldDescription
		for i := range e.description.Fields {
			f := &e.description.Fields[i]
			if f.Section == e.section && f.Key == key {
				schema = f
				break
			}
		}
		if schema == nil || schema.ReadOnly || e.description.Resource.Access != configmanager.AccessReadWrite {
			return false, fmt.Errorf("field is unknown or read-only")
		}
		value := ""
		if schema.Secret {
			if len(parts) != 1 {
				return false, fmt.Errorf("use set %s without a value to enter the secret privately", key)
			}
			var err error
			value, err = s.readConfigValue(ctx, input, "new value: ", true)
			if err != nil {
				return false, err
			}
		} else {
			if len(parts) != 2 {
				return false, fmt.Errorf("usage: set KEY VALUE")
			}
			value = strings.TrimSpace(parts[1])
			if strings.HasPrefix(value, "\"") {
				var err error
				value, err = strconv.Unquote(value)
				if err != nil {
					return false, fmt.Errorf("invalid quoted value")
				}
			}
		}
		if err := configmanager.CheckDraftValue(*schema, value); err != nil {
			return false, err
		}
		total := len(value) + len(e.section) + len(key)
		for _, c := range e.changes {
			if c.Section != e.section || c.Key != key {
				total += len(c.Value) + len(c.Section) + len(c.Key)
			}
		}
		if total > 256*1024 {
			return false, fmt.Errorf("draft exceeds size limit")
		}
		updated := false
		for i, c := range e.changes {
			if c.Section == e.section && c.Key == key {
				e.changes[i].Value = value
				updated = true
				break
			}
		}
		if !updated {
			if len(e.changes) >= configmanager.MaxPatchChanges {
				return false, fmt.Errorf("too many staged changes")
			}
			e.changes = append(e.changes, configmanager.FieldChange{Section: e.section, Key: key, Value: value})
		}
		fmt.Fprint(s.output, "Staged; end saves, cancel discards.\r\n")
	case "rollback":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: rollback")
		}
		if len(e.changes) > 0 {
			return false, fmt.Errorf("cancel staged changes before rollback")
		}
		answer, err := s.readConfigValue(ctx, input, "restore newest backup? [y/N]: ", false)
		if err != nil {
			return false, err
		}
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			return false, nil
		}
		if _, err = s.configs.Rollback(ctx, s.principal, e.resource); err != nil {
			return false, err
		}
		read, err := s.configs.Read(ctx, s.principal, e.resource)
		if err != nil {
			return false, err
		}
		e.base = read
	case "diff", "end":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: %s", command)
		}
		if len(e.changes) == 0 {
			if command == "end" {
				s.editing = nil
			}
			fmt.Fprint(s.output, "No changes.\r\n")
			return false, nil
		}
		if command == "diff" {
			for _, c := range e.changes {
				old := ""
				secret := false
				for _, f := range e.base.Fields {
					if f.Section == c.Section && f.Key == c.Key {
						old = f.Value
						secret = f.Redacted
					}
				}
				value := c.Value
				if secret {
					value = "[REDACTED]"
				}
				s.writeTerminalData([]byte(fmt.Sprintf("%s.%s: %s -> %s\n", c.Section, c.Key, old, value)))
			}
			return false, nil
		}
		preview, err := s.configs.Preview(ctx, s.principal, s.editPatch())
		if err != nil {
			return false, err
		}
		_, err = s.configs.Apply(ctx, s.principal, preview.Token)
		if err != nil {
			return false, err
		}
		s.editing = nil
		fmt.Fprint(s.output, "Configuration saved.\r\n")
	default:
		return false, fmt.Errorf("unknown configuration command; use ?")
	}
	return false, nil
}

func (s *Shell) editorHelp(fields []string) error {
	e := s.editing
	row := func(name, description string) {
		s.writeTerminalData([]byte(fmt.Sprintf("%-24s %s\n", name, description)))
	}
	if len(fields) == 0 {
		for _, r := range [][2]string{{"show", "Show values including staged changes; secrets are hidden"}, {"edit SECTION", "Select a registered section"}, {"set KEY VALUE", "Stage a value; omit VALUE for a secret"}, {"diff", "Show unsaved changes"}, {"next", "Return to the resource; keep staged changes"}, {"end", "Validate and save all changes, then leave configuration"}, {"rollback", "Restore the latest file backup; asks for confirmation"}, {"cancel", "Discard unsaved changes and leave configuration"}, {"exit", "Close session and discard unsaved changes"}} {
			row(r[0], r[1])
		}
		return nil
	}
	if fields[0] == "edit" && len(fields) == 1 {
		seen := map[string]bool{}
		for _, f := range e.description.Fields {
			name := f.Section
			if name == "" {
				name = "-"
			}
			if !seen[name] {
				row(name, "Registered section")
				seen[name] = true
			}
		}
		return nil
	}
	if fields[0] == "set" && e.inSection {
		for _, f := range e.description.Fields {
			if f.Section != e.section || f.ReadOnly {
				continue
			}
			if len(fields) > 1 && f.Key != fields[1] {
				continue
			}
			detail := string(f.Type)
			if f.Description != "" {
				detail += " — " + f.Description
			}
			if f.Min != nil {
				detail += fmt.Sprintf(" min=%d", *f.Min)
			}
			if f.Max != nil {
				detail += fmt.Sprintf(" max=%d", *f.Max)
			}
			if len(f.Enum) > 0 {
				detail += " allowed=" + strings.Join(f.Enum, "|")
			}
			if f.Secret {
				detail += " (hidden input)"
			}
			row(f.Key, detail)
		}
		return nil
	}
	return fmt.Errorf("use ?, edit ?, or set ?")
}

func (s *Shell) editorCompletion(before, prefix string) []string {
	fields := strings.Fields(before)
	index := len(fields) - 1
	if strings.HasSuffix(before, " ") {
		index++
	}
	if index <= 0 {
		return matching([]string{"show", "edit", "set", "diff", "next", "end", "cancel", "rollback", "exit"}, prefix)
	}
	e := s.editing
	values := []string{}
	if fields[0] == "edit" && index == 1 {
		seen := map[string]bool{}
		for _, f := range e.description.Fields {
			name := f.Section
			if name == "" {
				name = "-"
			}
			if !seen[name] {
				values = append(values, name)
				seen[name] = true
			}
		}
	}
	if fields[0] == "set" && e.inSection {
		for _, f := range e.description.Fields {
			if f.Section != e.section || f.ReadOnly {
				continue
			}
			if index == 1 {
				values = append(values, f.Key)
			}
			if index == 2 && len(fields) > 1 && f.Key == fields[1] && !f.Secret {
				values = append(values, f.Enum...)
				if f.Type == configmanager.FieldBoolean {
					values = append(values, "true", "false")
				}
			}
		}
	}
	return matching(values, prefix)
}
