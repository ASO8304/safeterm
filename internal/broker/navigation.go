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
	if len(s.configMenu) == 1 && s.configMenu[0] == "network" {
		s.networkHelpRow("interface", "Choose an interface to edit")
		s.networkHelpRow("route", "Choose a static route to edit")
	} else if err := s.configPath(ctx, s.configMenu, true); err != nil {
		return err
	}
	s.networkHelpRow("edit NAME", "Open one of the choices above; typing its name also works")
	s.networkHelpRow("back", "Return to the previous menu without saving")
	s.networkHelpRow("?", "Show this menu; command ? keeps your input")
	s.networkHelpRow("end", "Return to the main prompt")
	return nil
}

func (s *Shell) configurationMenuCommand(ctx context.Context, fields []string) (bool, bool, error) {
	command := fields[0]
	if command == "config" || command == "execute" || command == "whoami" || command == "clear" || command == "exit" || command == "logout" || command == "quit" {
		return false, false, nil
	}
	help := fields[len(fields)-1] == "?"
	if help {
		fields = fields[:len(fields)-1]
	}
	if len(fields) == 0 || (len(fields) == 1 && (command == "show" || command == "help")) {
		return true, false, s.showConfigurationMenu(ctx)
	}
	if command == "back" || command == "end" || command == "abort" || command == "cancel" {
		if len(fields) != 1 {
			return true, false, fmt.Errorf("usage: %s", command)
		}
		if help {
			s.networkHelpRow("<cr>", "Return without saving; back moves up one menu, end/abort returns to the main prompt")
			return true, false, nil
		}
		if command == "back" && len(s.configMenu) > 0 {
			return true, false, s.openConfigurationPath(ctx, s.configMenu[:len(s.configMenu)-1], false)
		}
		s.configMenu = nil
		return true, false, s.questionHelp(ctx, nil)
	}
	path := append([]string{}, s.configMenu...)
	if command == "edit" {
		if len(fields) == 1 && help {
			return true, false, s.showConfigurationMenu(ctx)
		}
		if len(fields) != 2 {
			return true, false, fmt.Errorf("usage: edit NAME; use ? to list choices")
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
	selecting := len(fields) > 0 && fields[0] == "edit"
	if len(completed) == 0 && !selecting {
		choices = append(choices, "?", "back", "edit", "show", "end", "abort", "exit")
	}
	return matching(choices, prefix), true
}
