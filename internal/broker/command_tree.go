package broker

import (
	"context"
	"fmt"
	"strings"

	"safeterm/internal/config"
	"safeterm/internal/resources"
)

// Presentation aliases resolve to the original signed operation ID. Catalog
// permissions, parameters and the gateway's independent authorization are intact.
func operationWords(name string) []string {
	if words, ok := resources.OperationWords(name); ok {
		return words
	}
	for _, action := range []string{"status", "restart", "start", "stop", "enable", "disable"} {
		if strings.HasPrefix(name, action+"_safeterm_") {
			return []string{"service", action, strings.TrimPrefix(name, action+"_safeterm_")}
		}
	}
	switch name {
	case "log":
		return []string{"logs"}
	case "memory_info":
		return []string{"memory"}
	case "disk_usage":
		return []string{"disk"}
	case "system_info":
		return []string{"system"}
	case "trace_host":
		return []string{"trace", "host"}
	case "trace_port":
		return []string{"trace", "port"}
	case "trace_filtered":
		return []string{"trace", "filtered"}
	}
	return []string{name}
}

func wordsStart(words, prefix []string) bool {
	if len(prefix) > len(words) {
		return false
	}
	for i, w := range prefix {
		if words[i] != w {
			return false
		}
	}
	return true
}

func (s *Shell) resolveOperation(fields []string) (config.OperationDefinition, []string, bool) {
	var selected config.OperationDefinition
	best := 0
	ambiguous := false
	for _, op := range s.catalog.AuthorizedOperations(s.principal.Roles) {
		path := operationWords(op.Name)
		if len(path) == best && best > 0 && wordsStart(fields, path) {
			ambiguous = true
		}
		if len(path) > best && wordsStart(fields, path) {
			ambiguous = false
			selected = op
			best = len(path)
		}
	}
	if ambiguous {
		return config.OperationDefinition{}, nil, false
	}
	if best > 0 {
		return selected, fields[best:], true
	}
	return selected, nil, false
}

func (s *Shell) operationTreeHelp(fields []string) error {
	row := func(name, description string) {
		s.writeTerminalData([]byte(fmt.Sprintf("%-24s %s\n", name, strings.Join(strings.Fields(description), " "))))
	}
	seen := map[string]bool{}
	found := false
	for _, op := range s.catalog.AuthorizedOperations(s.principal.Roles) {
		path := operationWords(op.Name)
		if len(path) <= len(fields) || !wordsStart(path, fields) {
			continue
		}
		name := path[len(fields)]
		if seen[name] {
			continue
		}
		seen[name] = true
		found = true
		desc := op.Description
		if len(path) > len(fields)+1 {
			desc = "Authorized " + name + " commands"
		}
		row(name, desc)
	}
	if !found {
		if op, _, ok := s.resolveOperation(fields); ok {
			s.describe(op)
			return nil
		}
		if len(fields) > 0 {
			return fmt.Errorf("command is unknown or unauthorized; use execute ?")
		}
		fmt.Fprint(s.output, "No authorized operations.\r\n")
	}
	return nil
}

func (s *Shell) treeCompletion(ctx context.Context, before, prefix string) ([]string, bool) {
	fields := strings.Fields(before)
	if len(fields) == 0 || fields[0] != "execute" {
		return nil, false
	}
	trailing := strings.HasSuffix(before, " ")
	path := fields[1:]
	parents := path
	if !trailing && len(parents) > 0 {
		parents = parents[:len(parents)-1]
	}
	values := []string{}
	seen := map[string]bool{}
	for _, op := range s.catalog.AuthorizedOperations(s.principal.Roles) {
		words := operationWords(op.Name)
		if len(words) > len(parents) && wordsStart(words, parents) {
			v := words[len(parents)]
			if !seen[v] {
				values = append(values, v)
				seen[v] = true
			}
		}
	}
	// A complete path also offers operation arguments, including the generic
	// trace operation alongside its host/port/filtered subcommands.
	if op, args, ok := s.resolveOperation(parents); ok {
		normalized := "execute " + op.Name + " "
		if len(args) > 0 {
			normalized += strings.Join(args, " ") + " "
		}
		normalized += prefix
		values = append(values, s.legacyCompletionCandidates(ctx, normalized, prefix)...)
	}
	return matching(values, prefix), true
}

// ValidateOperationPaths rejects ambiguous presentation names before activation.
func ValidateOperationPaths(cfg *config.Config) error {
	seen := map[string]string{}
	for _, op := range cfg.Operations {
		path := strings.Join(operationWords(op.Name), " ")
		if prior := seen[path]; prior != "" {
			return fmt.Errorf("terminal path %q conflicts between %s and %s", path, prior, op.Name)
		}
		seen[path] = op.Name
	}
	return nil
}
