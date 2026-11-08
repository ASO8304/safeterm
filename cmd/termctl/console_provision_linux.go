//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/identity"
)

const consoleLoginShell = "/usr/local/bin/safeterm-console"
const consoleLoginGroups = "safeterm-clients,safeterm-gateway"

var consoleAccountName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

type consoleProvisionHost interface {
	available(string) error
	password() ([]byte, error)
	lock() (func(), error)
	create(string) (string, error)
	reload() error
	activate(string, string, []byte) error
	disable(string, string) error
}

type consoleSystemHost struct{ out io.Writer }

func provisionLinuxConsoleUser(dir string, o userOptions, cfg *config.Config, out io.Writer) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("creating Linux Console accounts requires root; use sudo termctl user add console")
	}
	if filepath.Clean(dir) != defaultConfigDir {
		return fmt.Errorf("Linux account provisioning requires the installed /etc/safeterm configuration")
	}
	return provisionConsole(dir, o, cfg, consoleSystemHost{out}, out)
}

func consoleMappingPlan(dir string, o userOptions, cfg *config.Config) (identity.LocalUsers, map[string]bool, error) {
	if !consoleAccountName.MatchString(o.osUser) || o.osUser == "root" || o.osUser == "safeterm" || strings.HasPrefix(o.osUser, "safeterm-") {
		return identity.LocalUsers{}, nil, fmt.Errorf("choose an unused Linux username starting with a lowercase letter, followed by lowercase letters, digits, underscore or hyphen; maximum 32 characters; no service accounts")
	}
	if o.hashFile != "" || o.keyFile != "" {
		return identity.LocalUsers{}, nil, fmt.Errorf("Linux Console provisioning uses its own password prompt; do not provide SSH keys or SafeTerm password hashes")
	}
	known := map[string]bool{}
	for _, role := range cfg.Roles {
		known[role.Name] = true
	}
	users, err := identity.LoadLocalUsers(filepath.Join(dir, "access/console/linux-users.json"), known)
	if err != nil {
		return users, nil, err
	}
	for _, u := range users.Users {
		if u.OSUser == o.osUser {
			return users, nil, fmt.Errorf("Console username %q is already mapped; nothing changed; choose an unused username", o.osUser)
		}
		if u.UserID == o.id {
			return users, nil, fmt.Errorf("SafeTerm identity %q already exists; choose another --id or username", o.id)
		}
	}
	users.Users = append(users.Users, identity.LocalUser{OSUser: o.osUser, UserID: o.id, Roles: splitRoles(o.roles)})
	data, err := json.Marshal(users)
	if err != nil {
		return users, nil, err
	}
	if _, err = identity.ParseLocalUsers(data, known); err != nil {
		return users, nil, err
	}
	return users, known, nil
}

func provisionConsole(dir string, o userOptions, cfg *config.Config, h consoleProvisionHost, out io.Writer) error {
	if o.id == "" {
		o.id = o.osUser
	}
	if _, _, err := consoleMappingPlan(dir, o, cfg); err != nil {
		return err
	}
	if err := h.available(o.osUser); err != nil {
		return err
	}
	fmt.Fprintf(out, "New Linux Console user: %s. Setup restarts SafeTerm services; active sessions will need to reconnect.\n", o.osUser)
	password, err := h.password()
	defer clear(password)
	if err != nil {
		return err
	}
	if err = broker.ValidatePasswordComplexity(password); err != nil {
		return err
	}
	unlock, err := h.lock()
	if err != nil {
		return err
	}
	defer unlock()
	// Recheck after terminal input and lifecycle-lock acquisition. Existing
	// accounts, groups and home directories must never be adopted or modified.
	if err = h.available(o.osUser); err != nil {
		return err
	}
	users, known, err := consoleMappingPlan(dir, o, cfg)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Creating Linux Console account %s with login shell %s.\n", o.osUser, consoleLoginShell)
	uid, err := h.create(o.osUser)
	if err != nil {
		return fmt.Errorf("Linux account creation failed; no SafeTerm mapping or password was assigned: %w", err)
	}
	path := filepath.Join(dir, "access/console/linux-users.json")
	if err = validateAndWriteLocalUsers(path, users, known); err != nil {
		return fmt.Errorf("account %s remains password-locked; could not save Console mapping: %w", o.osUser, err)
	}
	rollback := func(cause error) error {
		current, readErr := identity.LoadLocalUsers(path, known)
		if readErr == nil {
			current.Users = slices.DeleteFunc(current.Users, func(u identity.LocalUser) bool { return u.UserID == o.id && u.OSUser == o.osUser })
			readErr = validateAndWriteLocalUsers(path, current, known)
		}
		lockErr := h.disable(o.osUser, uid)
		if readErr != nil || lockErr != nil {
			return fmt.Errorf("Console provisioning failed: %w; review newly created account %s (mapping cleanup: %v; password lock: %v)", cause, o.osUser, readErr, lockErr)
		}
		return fmt.Errorf("Console provisioning failed: %w; new account %s is password-locked and its mapping was removed; existing accounts were not changed", cause, o.osUser)
	}
	fmt.Fprintln(out, "Loading Console access.")
	if err = h.reload(); err != nil {
		return rollback(err)
	}
	// Unlock only after all SafeTerm boundaries have loaded the new UID.
	if err = h.activate(o.osUser, uid, password); err != nil {
		return rollback(err)
	}
	fmt.Fprintf(out, "Local Console user created: Linux=%s SafeTerm=%s roles=%s\nLog in to the machine as %s using its Linux password; SafeTerm opens without a second login.\n", o.osUser, o.id, o.roles, o.osUser)
	return nil
}

func (h consoleSystemHost) available(name string) error {
	if _, err := user.Lookup(name); err == nil {
		return fmt.Errorf("Linux user %q already exists; nothing changed; choose an unused username", name)
	} else if _, ok := err.(user.UnknownUserError); !ok {
		return err
	}
	if _, err := user.LookupGroup(name); err == nil {
		return fmt.Errorf("Linux group %q already exists; nothing changed; choose an unused username", name)
	} else if _, ok := err.(user.UnknownGroupError); !ok {
		return err
	}
	if _, err := os.Lstat(filepath.Join("/home", name)); !os.IsNotExist(err) {
		return fmt.Errorf("home path /home/%s already exists or cannot be checked; choose another username", name)
	}
	for _, name := range []string{"safeterm-clients", "safeterm-gateway"} {
		if _, err := user.LookupGroup(name); err != nil {
			return fmt.Errorf("required SafeTerm group %s is missing", name)
		}
	}
	for _, path := range []string{consoleLoginShell, "/usr/sbin/useradd", "/usr/sbin/chpasswd", "/usr/bin/passwd"} {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.Mode().IsRegular() || st.Uid != 0 || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0111 == 0 {
			return fmt.Errorf("unsafe or unavailable Console provisioning executable: %s", path)
		}
	}
	return nil
}
func (h consoleSystemHost) password() ([]byte, error) { return readInteractivePassword() }
func (h consoleSystemHost) lock() (func(), error)     { return catalogLock(false) }
func (h consoleSystemHost) create(name string) (string, error) {
	if err := execCommand("/usr/sbin/useradd", "--create-home", "--user-group", "--home-dir", filepath.Join("/home", name), "--shell", consoleLoginShell, "--groups", consoleLoginGroups, "--", name); err != nil {
		return "", err
	}
	u, err := user.Lookup(name)
	if err != nil || u.Uid == "0" {
		return "", fmt.Errorf("cannot verify newly created non-root account")
	}
	return u.Uid, nil
}
func (h consoleSystemHost) reload() error {
	if err := serviceCommand([]string{"restart", "all"}, h.out, h.out); err != nil {
		return err
	}
	sockets := []string{identity.SocketPath, "/run/safeterm-password-manager/manager.sock", "/run/safeterm-network-manager/manager.sock", "/run/safeterm-gateway/gateway.sock", "/run/safeterm-config-application/manager.sock", "/run/safeterm-config-database/manager.sock", "/run/safeterm-config-network/manager.sock"}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		ready := true
		for _, path := range sockets {
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeSocket == 0 {
				ready = false
				break
			}
		}
		if ready {
			units := []string{"--quiet", "is-active"}
			for _, name := range serviceOrder {
				units = append(units, serviceUnits[name])
			}
			_, err := catalogRun("/usr/bin/systemctl", units...)
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("SafeTerm services did not become ready after loading Console access")
}
func (h consoleSystemHost) activate(name, uid string, password []byte) error {
	u, err := user.Lookup(name)
	if err != nil || u.Uid != uid || uid == "0" {
		return fmt.Errorf("new Linux account identity changed")
	}
	return setLinuxConsolePassword(name, password)
}
func (h consoleSystemHost) disable(name, uid string) error {
	u, err := user.Lookup(name)
	if err != nil || u.Uid != uid || uid == "0" {
		return fmt.Errorf("new Linux account identity changed; refusing to modify it")
	}
	return execCommand("/usr/bin/passwd", "--lock", "--", name)
}
