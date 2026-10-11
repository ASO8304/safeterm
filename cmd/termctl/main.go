// Command termctl is the single local administrator entry point for SafeTerm.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
)

const defaultConfigDir = "/etc/safeterm"

var serviceUnits = map[string]string{
	"audit": "safeterm-audit.service", "gateway": "safeterm-gateway.service",
	"ssh": "safeterm-ssh-broker.service", "diagnostics": "safeterm-runner-diagnostics.service",
	"network": "safeterm-runner-network-observer.service", "logs": "safeterm-runner-log-reader.service",
	"services": "safeterm-runner-service-manager.service",
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "termctl:", err)
		os.Exit(1)
	}
}

func run(args []string, out, errOut io.Writer) error {
	configDir := defaultConfigDir
	if len(args) >= 2 && args[0] == "--config-dir" {
		configDir, args = args[1], args[2:]
	}
	if len(args) == 0 {
		return execCommand("/usr/local/bin/safeterm-console")
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		writeHelp(out)
		return nil
	}
	switch args[0] {
	case "completion":
		return writeCompletion(out, args[1:])
	case "catalog":
		return catalogCommand(configDir, args[1:], out)
	case "service":
		return serviceCommand(args[1:], out, errOut)
	case "config":
		return configCommand(configDir, args[1:], out)
	case "user":
		return userCommand(configDir, args[1:], out)
	case "shell":
		return execCommand("/usr/local/bin/safeterm-console", args[1:]...)
	default:
		return fmt.Errorf("unknown command %q; run 'termctl help'", args[0])
	}
}

func writeHelp(out io.Writer) {
	fmt.Fprintln(out, "SafeTerm control utility")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  termctl catalog list|describe OPERATION")
	fmt.Fprintln(out, "  termctl service status [NAME|all]")
	fmt.Fprintln(out, "  termctl service restart|start|stop NAME")
	fmt.Fprintln(out, "  termctl config list|describe RESOURCE")
	fmt.Fprintln(out, "  termctl user list")
	fmt.Fprintln(out, "  termctl shell")
	fmt.Fprintln(out, "  termctl completion bash|zsh")
	fmt.Fprintln(out, "Use Tab completion for commands, services, and catalog operations.")
}

func catalogCommand(dir string, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "list" {
		cfg, err := config.LoadConfig(dir)
		if err != nil {
			return err
		}
		for _, op := range cfg.Operations {
			fmt.Fprintf(out, "%-24s %s\n", op.Name, op.Description)
		}
		return nil
	}
	if args[0] != "describe" || len(args) != 2 {
		return fmt.Errorf("usage: termctl catalog list|describe OPERATION")
	}
	cfg, err := config.LoadConfig(dir)
	if err != nil {
		return err
	}
	for _, op := range cfg.Operations {
		if op.Name != args[1] {
			continue
		}
		fmt.Fprintf(out, "%s: %s\nprofile: %s\n", op.Name, op.Description, op.RunnerProfile)
		if op.Systemd != nil {
			fmt.Fprintf(out, "systemd: %s %s\n", op.Systemd.Action, op.Systemd.Unit)
		}
		for _, p := range op.Parameters {
			fmt.Fprintf(out, "parameter: %s (%s, required=%t, default=%s)\n", p.Name, p.Type, p.Required, p.Default)
		}
		return nil
	}
	return fmt.Errorf("operation %q was not found", args[1])
}

func serviceCommand(args []string, out, errOut io.Writer) error {
	if len(args) < 1 || (args[0] != "status" && args[0] != "start" && args[0] != "stop" && args[0] != "restart") {
		return fmt.Errorf("usage: termctl service status [NAME|all] | restart|start|stop NAME")
	}
	name := "all"
	if len(args) > 1 {
		name = args[1]
	}
	names := []string{name}
	if name == "all" {
		names = names[:0]
		for n := range serviceUnits {
			names = append(names, n)
		}
		sort.Strings(names)
	}
	for _, item := range names {
		unit, ok := serviceUnits[item]
		if !ok {
			return fmt.Errorf("unknown service %q; use tab completion", item)
		}
		cmd := exec.Command("/usr/bin/systemctl", "--no-pager", args[0], unit)
		cmd.Stdout, cmd.Stderr = out, errOut
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s %s: %w", args[0], item, err)
		}
	}
	return nil
}

func configCommand(dir string, args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "list" && args[0] != "describe") {
		return fmt.Errorf("usage: termctl config list|describe RESOURCE")
	}
	data, err := os.ReadFile(filepath.Join(dir, "config-manager", "resources.json"))
	if err != nil {
		return err
	}
	registry, err := configmanager.ParseRegistry(data)
	if err != nil {
		return err
	}
	if args[0] == "list" {
		for _, r := range registry.Resources {
			fmt.Fprintf(out, "%-24s %s\n", r.ID, r.Description)
		}
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("usage: termctl config describe RESOURCE")
	}
	for _, r := range registry.Resources {
		if r.ID == args[1] {
			fmt.Fprintf(out, "%s: %s\npath: %s\nformat: %s\n", r.ID, r.Description, r.Path, r.Format)
			for _, f := range r.Fields {
				fmt.Fprintf(out, "field: %s.%s (%s, required=%t, secret=%t)\n", f.Section, f.Key, f.Type, f.Required, f.Secret)
			}
			return nil
		}
	}
	return fmt.Errorf("resource %q was not found", args[1])
}

func userCommand(dir string, args []string, out io.Writer) error {
	if len(args) != 1 || args[0] != "list" {
		return fmt.Errorf("usage: termctl user list")
	}
	data, err := os.ReadFile(filepath.Join(dir, "ssh", "users.json"))
	if err != nil {
		return err
	}
	var users struct {
		Users []struct {
			UserID         string   `json:"user_id"`
			Roles          []string `json:"roles"`
			PasswordHash   string   `json:"password_hash"`
			AuthorizedKeys []string `json:"authorized_keys"`
		} `json:"users"`
	}
	if err := json.Unmarshal(data, &users); err != nil {
		return err
	}
	for _, u := range users.Users {
		methods := []string{}
		if u.PasswordHash != "" {
			methods = append(methods, "password")
		}
		if len(u.AuthorizedKeys) > 0 {
			methods = append(methods, "key")
		}
		fmt.Fprintf(out, "%-20s roles=%s auth=%s\n", u.UserID, strings.Join(u.Roles, ","), strings.Join(methods, "+"))
	}
	return nil
}

func execCommand(path string, args ...string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func writeCompletion(out io.Writer, args []string) error {
	if len(args) != 1 || (args[0] != "bash" && args[0] != "zsh") {
		return fmt.Errorf("usage: termctl completion bash|zsh")
	}
	commands := "catalog service config user shell completion help"
	services := "audit gateway ssh diagnostics network logs services all"
	if args[0] == "bash" {
		fmt.Fprintf(out, "_termctl_complete() { local cur prev; cur=\"${COMP_WORDS[COMP_CWORD]}\"; prev=\"${COMP_WORDS[COMP_CWORD-1]}\"; if [[ \\\"$prev\\\" == service ]]; then COMPREPLY=($(compgen -W \\\"%s\\\" -- \\\"$cur\\\")); elif [[ \\\"$prev\\\" == catalog || \\\"$prev\\\" == config ]]; then COMPREPLY=($(compgen -W \\\"list describe\\\" -- \\\"$cur\\\")); elif [[ \\\"$prev\\\" == user ]]; then COMPREPLY=($(compgen -W \\\"list\\\" -- \\\"$cur\\\")); else COMPREPLY=($(compgen -W \\\"%s\\\" -- \\\"$cur\\\")); fi; }; complete -F _termctl_complete termctl\n", services, commands)
		return nil
	}
	fmt.Fprintln(out, "#compdef termctl")
	fmt.Fprintln(out, "_arguments '1:command:(catalog service config user shell completion help)'")
	return nil
}
