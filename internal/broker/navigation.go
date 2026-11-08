package broker

import (
	"context"
	"fmt"
	"strings"
)

func (s *Shell) openConfigurationPath(ctx context.Context, path []string, help bool) error {
	if len(path) > 0 && path[0] == "network" && s.network != nil {
		fields := append([]string{"config"}, path...)
		if help {
			fields = append(fields, "?")
		}
		_, err := s.networkCommand(ctx, fields)
		return err
	}
	return s.configPath(ctx, path, help)
}

func (s *Shell) showConfigurationMenu(ctx context.Context) error {
	if err := s.showConfigurationChildren(ctx); err != nil {
		return err
	}
	s.networkHelpRow("edit PATH", "Open a child or relative path; typing the path directly also works")
	s.networkHelpRow("exit", exitDescription)
	s.networkHelpRow("cancel", "Return to the main prompt")
	s.networkHelpRow("clear", globalDescriptions["clear"])
	return nil
}

func (s *Shell) showConfigurationChildren(ctx context.Context) error {
	if len(s.configMenu) == 1 && s.configMenu[0] == "network" {
		s.networkHelpRow("interface", "Choose an interface to edit")
		s.networkHelpRow("route", "Choose a static route to edit")
	} else if err := s.configPath(ctx, s.configMenu, true); err != nil {
		return err
	}
	return nil
}

func (s *Shell) configurationMenuCommand(ctx context.Context, fields []string) (bool, bool, error) {
	command := fields[0]
	if command == "config" || command == "execute" || command == "clear" || command == "exit" || command == "logout" || command == "quit" {
		return false, false, nil
	}
	help := fields[len(fields)-1] == "?"
	if help {
		fields = fields[:len(fields)-1]
	}
	if len(fields) == 0 || (len(fields) == 1 && (command == "show" || command == "help")) {
		if command == "show" && !help {
			return true, false, s.showConfigurationChildren(ctx)
		}
		return true, false, s.showConfigurationMenu(ctx)
	}
	if command == "cancel" {
		if len(fields) != 1 {
			return true, false, fmt.Errorf("usage: %s", command)
		}
		if help {
			description := "Return to the main prompt and discard retained drafts"
			s.networkHelpRow("<cr>", description)
			return true, false, nil
		}
		s.discardLogicalUnder(nil)
		s.configMenu = nil
		return true, false, nil
	}
	path := append([]string{}, s.configMenu...)
	if command == "edit" {
		if len(fields) == 1 && help {
			return true, false, s.showConfigurationMenu(ctx)
		}
		if len(fields) < 2 {
			return true, false, fmt.Errorf("usage: edit PATH; use ? to list choices")
		}
		fields = fields[1:]
	}
	path = append(path, fields...)
	return true, false, s.openConfigurationPath(ctx, path, help)
}

func (s *Shell) configurationMenuCompletion(before, prefix string) ([]string, bool) {
	if s.configMenu == nil {
		return nil, false
	}
	fields := strings.Fields(before)
	if len(fields) > 0 && (fields[0] == "config" || fields[0] == "execute") {
		return nil, false
	}
	completed := fields
	if !strings.HasSuffix(before, " ") && len(completed) > 0 {
		completed = completed[:len(completed)-1]
	}
	if len(completed) > 0 && completed[0] == "edit" {
		completed = completed[1:]
	}
	path := append(append([]string{}, s.configMenu...), completed...)
	var choices []string
	if len(path) == 1 && path[0] == "network" {
		choices = []string{"interface", "route"}
	} else {
		line := "config " + strings.Join(path, " ")
		if len(path) > 0 {
			line += " "
		}
		choices = s.configCompletion(context.Background(), line+prefix, prefix)
		if len(path) == 0 && s.network != nil {
			choices = append(choices, "network")
		}
	}
	return matching(choices, prefix), true
}
