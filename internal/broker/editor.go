package broker

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
)

const maxShellHistory = 100

func (s *Shell) readLine(ctx context.Context, input <-chan byte, prompt string, secret, useHistory bool) (string, error) {
	line := make([]byte, 0, 128)
	cursor, historyIndex := 0, len(s.history)
	draft := ""
	escape := ""
	fmt.Fprint(s.output, prompt)
	redraw := func() {
		display := string(line)
		if secret {
			display = ""
		}
		fmt.Fprintf(s.output, "\r%s%s\x1b[K", prompt, display)
		if remaining := len(line) - cursor; remaining > 0 && !secret {
			fmt.Fprintf(s.output, "\x1b[%dD", remaining)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case b, ok := <-input:
			if !ok {
				return "", io.EOF
			}
			if escape != "" {
				escape += string(b)
				if len(escape) > 16 || (b >= 0x40 && b <= 0x7e && b != '[' && b != 'O') {
					var changed bool
					line, cursor, historyIndex, draft, changed = s.handleEscape(escape, line, cursor, historyIndex, draft, useHistory)
					if changed {
						redraw()
					}
					escape = ""
				}
				continue
			}
			if b != '\n' {
				s.skipLF = false
			}
			switch b {
			case 0x1b:
				escape = "\x1b"
			case 0x01: // Ctrl+A
				cursor = 0
				redraw()
			case 0x05: // Ctrl+E
				cursor = len(line)
				redraw()
			case 0x0c: // Ctrl+L
				fmt.Fprint(s.output, "\x1b[2J\x1b[H")
				redraw()
			case 0x15: // Ctrl+U
				line = append([]byte(nil), line[cursor:]...)
				cursor = 0
				redraw()
			case 0x17: // Ctrl+W
				start := cursor
				for start > 0 && line[start-1] == ' ' {
					start--
				}
				for start > 0 && line[start-1] != ' ' {
					start--
				}
				line = append(line[:start], line[cursor:]...)
				cursor = start
				redraw()
			case 0x03:
				fmt.Fprint(s.output, "^C\r\n")
				if secret || !useHistory {
					return "", fmt.Errorf("input cancelled")
				}
				return "", nil
			case 0x04:
				if len(line) == 0 {
					return "", io.EOF
				}
				if cursor < len(line) {
					line = append(line[:cursor], line[cursor+1:]...)
					redraw()
				}
			case '\r', '\n':
				if b == '\n' && s.skipLF {
					s.skipLF = false
					continue
				}
				if b == '\r' {
					s.skipLF = true
				}
				fmt.Fprint(s.output, "\r\n")
				value := string(line)
				if useHistory && value != "" && (len(s.history) == 0 || s.history[len(s.history)-1] != value) {
					s.history = append(s.history, value)
					if len(s.history) > maxShellHistory {
						s.history = s.history[len(s.history)-maxShellHistory:]
					}
				}
				return value, nil
			case '\t':
				if !secret && useHistory {
					line, cursor = s.completeLine(ctx, line, cursor, prompt)
					redraw()
				}
			case 0x7f, 0x08:
				if cursor > 0 {
					line = append(line[:cursor-1], line[cursor:]...)
					cursor--
					redraw()
				}
			default:
				if b >= 0x20 && b <= 0x7e {
					if len(line) >= maxShellLineBytes {
						fmt.Fprint(s.output, "\a")
						continue
					}
					line = append(line, 0)
					copy(line[cursor+1:], line[cursor:])
					line[cursor] = b
					cursor++
					redraw()
				}
			}
		}
	}
}

func (s *Shell) handleEscape(sequence string, line []byte, cursor, historyIndex int, draft string, useHistory bool) ([]byte, int, int, string, bool) {
	changed := true
	switch sequence {
	case "\x1b[D", "\x1bOD":
		if cursor > 0 {
			cursor--
		}
	case "\x1b[C", "\x1bOC":
		if cursor < len(line) {
			cursor++
		}
	case "\x1b[H", "\x1bOH", "\x1b[1~", "\x1b[7~":
		cursor = 0
	case "\x1b[F", "\x1bOF", "\x1b[4~", "\x1b[8~":
		cursor = len(line)
	case "\x1b[A", "\x1bOA":
		if useHistory && len(s.history) > 0 {
			if historyIndex == len(s.history) {
				draft = string(line)
			}
			if historyIndex > 0 {
				historyIndex--
				line = []byte(s.history[historyIndex])
				cursor = len(line)
			}
		}
	case "\x1b[B", "\x1bOB":
		if useHistory && historyIndex < len(s.history) {
			historyIndex++
			if historyIndex == len(s.history) {
				line = []byte(draft)
			} else {
				line = []byte(s.history[historyIndex])
			}
			cursor = len(line)
		}
	case "\x1b[3~":
		if cursor < len(line) {
			copy(line[cursor:], line[cursor+1:])
			line = line[:len(line)-1]
		}
	default:
		changed = false
	}
	return line, cursor, historyIndex, draft, changed
}

func (s *Shell) completeLine(ctx context.Context, line []byte, cursor int, prompt string) ([]byte, int) {
	start := cursor
	for start > 0 && line[start-1] != ' ' {
		start--
	}
	prefix := string(line[start:cursor])
	candidates := s.completionCandidates(ctx, string(line[:cursor]), prefix)
	if len(candidates) == 0 {
		fmt.Fprint(s.output, "\a")
		return line, cursor
	}
	sort.Strings(candidates)
	replacement := commonPrefix(candidates)
	if len(candidates) == 1 {
		replacement = candidates[0]
	}
	if replacement == prefix && len(candidates) > 1 {
		fmt.Fprint(s.output, "\r\n"+strings.Join(candidates, "  ")+"\r\n")
		fmt.Fprint(s.output, prompt)
	}
	updated := make([]byte, 0, len(line)-len(prefix)+len(replacement)+1)
	updated = append(updated, line[:start]...)
	updated = append(updated, replacement...)
	newCursor := len(updated)
	updated = append(updated, line[cursor:]...)
	if len(candidates) == 1 && newCursor == len(updated) && (start == 0 || !strings.HasSuffix(replacement, "=")) {
		updated = append(updated, ' ')
		newCursor++
	}
	return updated, newCursor
}

func commonPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := values[0]
	for _, value := range values[1:] {
		for !strings.HasPrefix(value, prefix) && prefix != "" {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

func matching(values []string, prefix string) []string {
	result := []string{}
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			result = append(result, value)
		}
	}
	return result
}

func (s *Shell) completionCandidates(ctx context.Context, before, prefix string) []string {
	fields := strings.Fields(before)
	trailing := strings.HasSuffix(before, " ")
	index := len(fields) - 1
	if trailing {
		index++
	}
	if index <= 0 {
		return matching([]string{"clear", "config", "describe", "exit", "help", "list", "execute", "whoami"}, prefix)
	}
	command := fields[0]
	if (command == "execute" || command == "describe") && index == 1 {
		names := []string{}
		for _, operation := range s.catalog.AuthorizedOperations(s.principal.Roles) {
			names = append(names, operation.Name)
		}
		return matching(names, prefix)
	}
	if command == "help" && index == 1 {
		values := []string{"config", "keys", "execute"}
		for _, operation := range s.catalog.AuthorizedOperations(s.principal.Roles) {
			values = append(values, operation.Name)
		}
		return matching(values, prefix)
	}
	if command == "execute" && index >= 2 && len(fields) >= 2 {
		operation, ok := s.catalog.AuthorizedOperation(fields[1], s.principal.Roles)
		if !ok {
			return nil
		}
		used := map[string]bool{}
		for _, field := range fields[2:] {
			if parts := strings.SplitN(field, "=", 2); len(parts) > 0 {
				used[parts[0]] = true
			}
		}
		values := []string{}
		position := index - 2
		for parameterIndex, parameter := range operation.Parameters {
			if !used[parameter.Name] || strings.HasPrefix(prefix, parameter.Name+"=") {
				values = append(values, parameter.Name+"=")
			}
			if parameterIndex == position && !strings.Contains(prefix, "=") {
				for alias := range parameter.ValueMap {
					values = append(values, alias)
				}
			}
			if strings.HasPrefix(prefix, parameter.Name+"=") {
				for alias := range parameter.ValueMap {
					values = append(values, parameter.Name+"="+alias)
				}
			}
			if parameter.Default != "" && !strings.Contains(prefix, "=") {
				values = append(values, parameter.Name+"="+parameter.Default)
			}
			if !strings.Contains(prefix, "=") {
				values = append(values, parameter.Aliases...)
			}
		}
		sort.Strings(values)
		return matching(values, prefix)
	}
	if command != "config" || s.configs == nil {
		return nil
	}
	if index == 1 {
		return matching([]string{"apply", "describe", "edit", "list", "rollback", "show"}, prefix)
	}
	if index == 2 && len(fields) >= 2 && fields[1] != "apply" && fields[1] != "list" {
		items, err := s.configs.List(ctx, s.principal)
		if err != nil {
			return nil
		}
		values := []string{}
		for _, item := range items {
			values = append(values, item.ID)
		}
		return matching(values, prefix)
	}
	if len(fields) >= 3 && fields[1] == "edit" && (index == 3 || index == 4) {
		description, err := s.configs.Describe(ctx, s.principal, fields[2])
		if err != nil {
			return nil
		}
		values, seen := []string{}, map[string]bool{}
		if index == 3 {
			for _, field := range description.Fields {
				section := field.Section
				if section == "" {
					section = "-"
				}
				if !seen[section] {
					values = append(values, section)
					seen[section] = true
				}
			}
		} else if len(fields) >= 4 {
			section := fields[3]
			if section == "-" {
				section = ""
			}
			for _, field := range description.Fields {
				if field.Section == section && !field.ReadOnly {
					values = append(values, field.Key)
				}
			}
		}
		return matching(values, prefix)
	}
	return nil
}
