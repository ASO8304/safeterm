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

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/deployment"
)

const defaultConfigDir = "/etc/safeterm"

var serviceUnits = map[string]string{
	"audit": "safeterm-audit.service", "gateway": "safeterm-gateway.service",
	"ssh": "safeterm-ssh-broker.service", "diagnostics": "safeterm-runner-diagnostics.service",
	"network": "safeterm-runner-network-observer.service", "logs": "safeterm-runner-log-reader.service",
	"services": "safeterm-runner-service-manager.service",
	"config-application": "safeterm-config-manager-application.service",
	"config-database":    "safeterm-config-manager-database.service",
	"config-network":     "safeterm-config-manager-network.service",
}

// serviceOrder is the dependency order used for bulk lifecycle operations.
// Stopping and disabling use the reverse order so dependents leave first.
var serviceOrder = []string{
	"audit", "diagnostics", "network", "logs", "services", "gateway",
	"config-application", "config-database", "config-network", "ssh",
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
	case "role", "roles":
		return roleCommand(configDir, args[1:], out)
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
	fmt.Fprintln(out, "  termctl catalog prepare --source DIR --output DIR")
	fmt.Fprintln(out, "  termctl role list|describe ROLE")
	fmt.Fprintln(out, "  termctl service status [NAME|all]")
	fmt.Fprintln(out, "  termctl service start|stop|restart|enable|disable [NAME|all]")
	fmt.Fprintln(out, "  termctl config list|describe RESOURCE")
	fmt.Fprintln(out, "  termctl user list")
	fmt.Fprintln(out, "  termctl user add ssh|console [--id ID] [--os-user OS_USER] --roles ROLE1,ROLE2 [--password-hash-file FILE] [--authorized-key-file FILE]")
	fmt.Fprintln(out, "  termctl user auth ssh|console [--id ID] [--os-user OS_USER] [--password-hash-file FILE] [--authorized-key-file FILE]")
	fmt.Fprintln(out, "  termctl user password ssh --id ID")
	fmt.Fprintln(out, "  termctl user password console --os-user OS_USER")
	fmt.Fprintln(out, "  termctl user del ssh|console --id ID|--os-user OS_USER")
	fmt.Fprintln(out, "  termctl shell")
	fmt.Fprintln(out, "  termctl completion bash|zsh")
	fmt.Fprintln(out, "Use Tab completion for commands, services, and catalog operations.")
}

func catalogCommand(dir string, args []string, out io.Writer) error {
	if len(args) > 0 && (args[0] == "prepare" || args[0] == "regenerate-hashes") {
		source, output := dir, ""
		for i := 1; i < len(args); i++ {
			if i+1 >= len(args) {
				return fmt.Errorf("catalog %s option %q requires a value", args[0], args[i])
			}
			switch args[i] {
			case "--source":
				source = args[i+1]
			case "--output":
				output = args[i+1]
			default:
				return fmt.Errorf("unknown catalog option %q", args[i])
			}
			i++
		}
		if output == "" {
			return fmt.Errorf("usage: termctl catalog prepare --source DIR --output DIR")
		}
		if err := deployment.PrepareConfig(source, output); err != nil {
			return err
		}
		fmt.Fprintf(out, "prepared unsigned catalog with refreshed executable hashes: %s\n", output)
		return nil
	}
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

func roleCommand(dir string, args []string, out io.Writer) error {
	cfg, err := config.LoadConfig(dir)
	if err != nil {
		return err
	}
	policies := map[string][]string{}
	for _, policy := range cfg.Policies {
		policies[policy.Role] = append(policies[policy.Role], policy.Operations...)
	}
	if len(args) == 0 || args[0] == "list" {
		for _, role := range cfg.Roles {
			fmt.Fprintf(out, "%-20s operations=%d\n", role.Name, len(policies[role.Name]))
		}
		return nil
	}
	if args[0] != "describe" || len(args) != 2 {
		return fmt.Errorf("usage: termctl role list|describe ROLE")
	}
	for _, role := range cfg.Roles {
		if role.Name == args[1] {
			sort.Strings(policies[role.Name])
			fmt.Fprintf(out, "%s: %s\n", role.Name, role.Description)
			for _, operation := range policies[role.Name] {
				fmt.Fprintf(out, "  - %s\n", operation)
			}
			return nil
		}
	}
	return fmt.Errorf("role %q was not found", args[1])
}

func serviceCommand(args []string, out, errOut io.Writer) error {
	if len(args) < 1 || !isServiceAction(args[0]) || len(args) > 2 {
		return fmt.Errorf("usage: termctl service status|start|stop|restart|enable|disable [NAME|all]")
	}
	name := "all"
	if len(args) > 1 {
		name = args[1]
	}
	names, err := selectedServices(name, args[0])
	if err != nil {
		return err
	}
	var failures []string
	for _, item := range names {
		unit := serviceUnits[item]
		cmd := exec.Command("/usr/bin/systemctl", "--no-pager", args[0], unit)
		cmd.Stdout, cmd.Stderr = out, errOut
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(errOut, "termctl service %s %s failed: %v\n", args[0], item, err)
			failures = append(failures, item)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("service action %s failed for: %s", args[0], strings.Join(failures, ", "))
	}
	return nil
}

func isServiceAction(action string) bool {
	switch action {
	case "status", "start", "stop", "restart", "enable", "disable":
		return true
	default:
		return false
	}
}

func selectedServices(name, action string) ([]string, error) {
	if name != "all" {
		if _, ok := serviceUnits[name]; !ok {
			return nil, fmt.Errorf("unknown service %q; use termctl help", name)
		}
		return []string{name}, nil
	}
	names := append([]string(nil), serviceOrder...)
	if action == "stop" || action == "disable" {
		for left, right := 0, len(names)-1; left < right; left, right = left+1, right-1 {
			names[left], names[right] = names[right], names[left]
		}
	}
	return names, nil
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
	if len(args) == 0 || args[0] == "list" {
		return listUsers(dir, out)
	}
	if args[0] == "password" {
		if len(args) < 2 || (args[1] != "ssh" && args[1] != "console") {
			return fmt.Errorf("usage: termctl user password ssh|console --id ID|--os-user OS_USER")
		}
		o, err := parseUserOptions(args[2:])
		if err != nil {
			return err
		}
		return setUserPassword(dir, args[1], o, out)
	}
	if args[0] != "add" && args[0] != "del" && args[0] != "auth" {
		return fmt.Errorf("usage: termctl user list | add|del|auth ssh|console [options]")
	}
	if len(args) < 2 || (args[1] != "ssh" && args[1] != "console") {
		return fmt.Errorf("user type must be ssh or console")
	}
	kind := args[1]
	options, err := parseUserOptions(args[2:])
	if err != nil {
		return err
	}
	cfg, err := config.LoadConfig(dir)
	if err != nil {
		return err
	}
	if options.id == "" && kind == "ssh" {
		return fmt.Errorf("--id is required")
	}
	if kind == "console" && options.osUser == "" {
		return fmt.Errorf("--os-user is required")
	}
	if args[0] == "del" {
		return deleteUser(dir, kind, options, cfg, out)
	}
	if args[0] == "add" && options.roles == "" {
		return fmt.Errorf("--roles is required")
	}
	return upsertUser(dir, kind, args[0] == "auth", options, cfg, out)
}

type userOptions struct{ id, osUser, roles, hashFile, keyFile string }

func parseUserOptions(args []string) (userOptions, error) {
	var o userOptions
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			return o, fmt.Errorf("option %q requires a value", args[i])
		}
		value := args[i+1]
		switch args[i] {
		case "--id":
			o.id = value
		case "--os-user":
			o.osUser = value
		case "--roles":
			o.roles = value
		case "--password-hash-file":
			o.hashFile = value
		case "--authorized-key-file":
			o.keyFile = value
		default:
			return o, fmt.Errorf("unknown user option %q", args[i])
		}
		i++
	}
	return o, nil
}

func listUsers(dir string, out io.Writer) error {
	var ssh broker.SSHIdentityConfig
	if data, err := os.ReadFile(filepath.Join(dir, "ssh", "users.json")); err == nil {
		if err := json.Unmarshal(data, &ssh); err != nil {
			return err
		}
		for _, u := range ssh.Users {
			methods := []string{}
			if u.PasswordHash != "" {
				methods = append(methods, "password")
			}
			if len(u.AuthorizedKeys) > 0 {
				methods = append(methods, "key")
			}
			fmt.Fprintf(out, "ssh/%-16s roles=%s auth=%s\n", u.UserID, strings.Join(u.Roles, ","), strings.Join(methods, "+"))
		}
	}
	var console broker.ConsoleIdentityConfig
	if data, err := os.ReadFile(filepath.Join(dir, "console", "users.json")); err == nil {
		if err := json.Unmarshal(data, &console); err != nil {
			return err
		}
		for _, u := range console.Users {
			fmt.Fprintf(out, "console/%-11s os-user=%s roles=%s\n", u.UserID, u.OSUser, strings.Join(u.Roles, ","))
		}
	}
	return nil
}

func upsertUser(dir string, kind string, auth bool, o userOptions, cfg *config.Config, out io.Writer) error {
	roles := splitRoles(o.roles)
	known := map[string]bool{}
	for _, role := range cfg.Roles {
		known[role.Name] = true
	}
	for _, role := range roles {
		if !known[role] {
			return fmt.Errorf("unknown role %q", role)
		}
	}
	if kind == "ssh" {
		path := filepath.Join(dir, "ssh", "users.json")
		var data broker.SSHIdentityConfig
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
		index := -1
		for i := range data.Users {
			if data.Users[i].UserID == o.id {
				index = i
				break
			}
		}
		if !auth && index >= 0 {
			return fmt.Errorf("SSH user %q already exists", o.id)
		}
		if auth && index < 0 {
			return fmt.Errorf("SSH user %q does not exist", o.id)
		}
		user := broker.SSHUser{UserID: o.id, Roles: roles}
		if index >= 0 {
			user = data.Users[index]
		}
		if len(roles) > 0 {
			user.Roles = roles
		}
		if o.hashFile != "" {
			hash, err := readSingleLine(o.hashFile)
			if err != nil {
				return err
			}
			if err := broker.ValidatePasswordHash(hash); err != nil {
				return err
			}
			user.PasswordHash = hash
		}
		if o.keyFile != "" {
			key, err := os.ReadFile(o.keyFile)
			if err != nil {
				return err
			}
			user.AuthorizedKeys = nonEmptyLines(string(key))
		}
		if user.PasswordHash == "" && len(user.AuthorizedKeys) == 0 {
			if !auth {
				hash, err := readInteractivePasswordHash()
				if err != nil {
					return err
				}
				user.PasswordHash = hash
			} else {
				return fmt.Errorf("SSH user requires --password-hash-file or --authorized-key-file")
			}
		}
		if index < 0 {
			data.Users = append(data.Users, user)
		} else {
			data.Users[index] = user
		}
		if _, err := broker.NewSSHAuthenticator(data, known); err != nil {
			return err
		}
		if err := writeJSONAtomic(path, data, 0640); err != nil {
			return err
		}
		fmt.Fprintf(out, "SSH user %s updated\n", o.id)
		return nil
	}
	path := filepath.Join(dir, "console", "users.json")
	var data broker.ConsoleIdentityConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	index := -1
	for i := range data.Users {
		if data.Users[i].OSUser == o.osUser {
			index = i
			break
		}
	}
	if !auth && index >= 0 {
		return fmt.Errorf("console user %q already exists", o.osUser)
	}
	if auth && index < 0 {
		return fmt.Errorf("console user %q does not exist", o.osUser)
	}
	entry := broker.ConsoleIdentity{OSUser: o.osUser, UserID: o.id, Roles: roles}
	if index >= 0 {
		entry = data.Users[index]
	}
	if o.id != "" {
		entry.UserID = o.id
	}
	if len(roles) > 0 {
		entry.Roles = roles
	}
	if index < 0 {
		data.Users = append(data.Users, entry)
	} else {
		data.Users[index] = entry
	}
	encoded, _ := json.Marshal(data)
	for _, u := range data.Users {
		if _, err := broker.ParseConsolePrincipal(encoded, u.OSUser, known); err != nil {
			return err
		}
	}
	if err := writeJSONAtomic(path, data, 0640); err != nil {
		return err
	}
	fmt.Fprintf(out, "console user %s updated\n", o.osUser)
	return nil
}

func deleteUser(dir, kind string, o userOptions, cfg *config.Config, out io.Writer) error {
	_ = cfg
	if kind == "ssh" {
		path := filepath.Join(dir, "ssh", "users.json")
		var data broker.SSHIdentityConfig
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
		next := data.Users[:0]
		found := false
		for _, u := range data.Users {
			if u.UserID == o.id {
				found = true
				continue
			}
			next = append(next, u)
		}
		if !found {
			return fmt.Errorf("SSH user %q does not exist", o.id)
		}
		data.Users = next
		if len(data.Users) == 0 {
			return fmt.Errorf("cannot delete the last SSH user")
		}
		known := map[string]bool{}
		for _, role := range cfg.Roles {
			known[role.Name] = true
		}
		if _, err := broker.NewSSHAuthenticator(data, known); err != nil {
			return err
		}
		if err := writeJSONAtomic(path, data, 0640); err != nil {
			return err
		}
		fmt.Fprintf(out, "SSH user %s deleted\n", o.id)
		return nil
	}
	path := filepath.Join(dir, "console", "users.json")
	var data broker.ConsoleIdentityConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	next := data.Users[:0]
	found := false
	for _, u := range data.Users {
		if u.OSUser == o.osUser {
			found = true
			continue
		}
		next = append(next, u)
	}
	if !found {
		return fmt.Errorf("console user %q does not exist", o.osUser)
	}
	if len(next) == 0 {
		return fmt.Errorf("cannot delete the last console user")
	}
	data.Users = next
	known := map[string]bool{}
	for _, role := range cfg.Roles {
		known[role.Name] = true
	}
	encoded, _ := json.Marshal(data)
	for _, u := range data.Users {
		if _, err := broker.ParseConsolePrincipal(encoded, u.OSUser, known); err != nil {
			return err
		}
	}
	if err := writeJSONAtomic(path, data, 0640); err != nil {
		return err
	}
	fmt.Fprintf(out, "console user %s deleted\n", o.osUser)
	return nil
}

func setUserPassword(dir, kind string, o userOptions, out io.Writer) error {
	if kind == "ssh" && o.id == "" {
		return fmt.Errorf("--id is required")
	}
	if kind == "console" && o.osUser == "" {
		return fmt.Errorf("--os-user is required")
	}
	password, err := readInteractivePassword()
	if err != nil {
		return err
	}
	defer clear(password)
	if err := broker.ValidatePasswordComplexity(password); err != nil {
		return err
	}
	if kind == "console" {
		cmd := exec.Command("/usr/sbin/chpasswd", "-c", "SHA512")
		cmd.Stdin = strings.NewReader(o.osUser + ":" + string(password) + "\n")
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("set console password: %w", err)
		}
		fmt.Fprintf(out, "console password updated for %s\n", o.osUser)
		return nil
	}
	path := filepath.Join(dir, "ssh", "users.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var data broker.SSHIdentityConfig
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	found := false
	hash, err := bcrypt.GenerateFromPassword(password, broker.PasswordHashCost)
	if err != nil {
		return err
	}
	for i := range data.Users {
		if data.Users[i].UserID == o.id {
			data.Users[i].PasswordHash = string(hash)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("SSH user %q does not exist", o.id)
	}
	cfg, err := config.LoadConfig(dir)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, role := range cfg.Roles {
		known[role.Name] = true
	}
	if _, err := broker.NewSSHAuthenticator(data, known); err != nil {
		return err
	}
	if err := writeJSONAtomic(path, data, 0640); err != nil {
		return err
	}
	fmt.Fprintf(out, "SSH password updated for %s\n", o.id)
	return nil
}

func readInteractivePassword() ([]byte, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, fmt.Errorf("an interactive terminal is required to set a password")
	}
	fmt.Fprint(os.Stderr, "New password: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	fmt.Fprint(os.Stderr, "Confirm password: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		clear(first)
		return nil, err
	}
	if string(first) != string(second) {
		clear(first)
		clear(second)
		return nil, fmt.Errorf("passwords do not match")
	}
	clear(second)
	return first, nil
}

func readInteractivePasswordHash() (string, error) {
	password, err := readInteractivePassword()
	if err != nil {
		return "", err
	}
	defer clear(password)
	if err := broker.ValidatePasswordComplexity(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword(password, broker.PasswordHashCost)
	if err != nil {
		return "", fmt.Errorf("password hashing failed")
	}
	return string(hash), nil
}

func splitRoles(value string) []string {
	var roles []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			roles = append(roles, item)
		}
	}
	sort.Strings(roles)
	return roles
}
func readSingleLine(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
func nonEmptyLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".termctl-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
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
	services := "audit gateway ssh diagnostics network logs services config-application config-database config-network all"
	serviceActions := "status start stop restart enable disable"
	if args[0] == "bash" {
		fmt.Fprintf(out, "_termctl_complete() { local cur prev; cur=\"${COMP_WORDS[COMP_CWORD]}\"; prev=\"${COMP_WORDS[COMP_CWORD-1]}\"; if [[ \\\"$prev\\\" == service ]]; then COMPREPLY=($(compgen -W \\\"%s\\\" -- \\\"$cur\\\")); elif [[ \\\"$prev\\\" == status || \\\"$prev\\\" == start || \\\"$prev\\\" == stop || \\\"$prev\\\" == restart || \\\"$prev\\\" == enable || \\\"$prev\\\" == disable ]]; then COMPREPLY=($(compgen -W \\\"%s\\\" -- \\\"$cur\\\")); elif [[ \\\"$prev\\\" == catalog || \\\"$prev\\\" == config ]]; then COMPREPLY=($(compgen -W \\\"list describe\\\" -- \\\"$cur\\\")); elif [[ \\\"$prev\\\" == user ]]; then COMPREPLY=($(compgen -W \\\"list add auth del password\\\" -- \\\"$cur\\\")); else COMPREPLY=($(compgen -W \\\"%s\\\" -- \\\"$cur\\\")); fi; }; complete -F _termctl_complete termctl\n", serviceActions, services, commands)
		return nil
	}
	fmt.Fprintln(out, "#compdef termctl")
	fmt.Fprintf(out, "_arguments '1:command:(%s)' '2:service action:(%s)' '3:service:(%s)'\n", commands, serviceActions, services)
	return nil
}
