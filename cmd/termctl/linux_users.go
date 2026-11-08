package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"

	"safeterm/internal/config"
	"safeterm/internal/identity"
)

// This edits mappings only. It never creates Linux accounts, changes passwords
// or shells, or grants group membership implicitly.
func linuxConsoleUser(dir, action string, o userOptions, cfg *config.Config, out io.Writer) error {
	if o.hashFile != "" || o.keyFile != "" {
		return fmt.Errorf("Linux Console users authenticate through Linux; do not provide SafeTerm password hashes or SSH keys")
	}
	if o.osUser == "root" || o.osUser == "safeterm" || strings.HasPrefix(o.osUser, "safeterm-") {
		return fmt.Errorf("use an individual Linux account, not a service account")
	}
	if action != "del" {
		account, err := user.Lookup(o.osUser)
		if err != nil || account.Uid == "0" {
			return fmt.Errorf("Linux account %q must exist and be non-root", o.osUser)
		}
	}
	known := map[string]bool{}
	for _, r := range cfg.Roles {
		known[r.Name] = true
	}
	path := filepath.Join(dir, "access/console/linux-users.json")
	users, err := identity.LoadLocalUsers(path, known)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(users.Users, func(u identity.LocalUser) bool { return u.UserID == o.id })
	switch action {
	case "add":
		if index >= 0 {
			return fmt.Errorf("Linux Console user already exists")
		}
		users.Users = append(users.Users, identity.LocalUser{OSUser: o.osUser, UserID: o.id, Roles: splitRoles(o.roles)})
	case "auth":
		if index < 0 {
			return fmt.Errorf("Linux Console user does not exist")
		}
		users.Users[index].OSUser = o.osUser
		if o.roles != "" {
			users.Users[index].Roles = splitRoles(o.roles)
		}
	case "del":
		if index < 0 || users.Users[index].OSUser != o.osUser {
			return fmt.Errorf("Linux Console mapping does not exist")
		}
		users.Users = slices.Delete(users.Users, index, index+1)
	default:
		return fmt.Errorf("unsupported Linux Console action")
	}
	// Reuse the strict validator before publishing root-managed authorization.
	if err := validateAndWriteLocalUsers(path, users, known); err != nil {
		return err
	}
	fmt.Fprintf(out, "Linux Console mapping %s %s. Restart SafeTerm services to load permitted UIDs; mapping removal and role changes revoke existing sessions immediately.\n", o.id, action)
	if action != "del" {
		fmt.Fprintf(out, "Linux account %s needs safeterm-clients,safeterm-gateway groups. After a new Linux login, run termctl without sudo. Use /usr/local/bin/safeterm-console as its login shell for restricted access.\n", o.osUser)
	}
	return nil
}

func validateAndWriteLocalUsers(path string, users identity.LocalUsers, known map[string]bool) error {
	data, err := json.Marshal(users)
	if err != nil {
		return err
	}
	if _, err := identity.ParseLocalUsers(data, known); err != nil {
		return err
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		// Publish a complete new file without overwriting a concurrently created
		// mapping. The installed parent directory is writable only by root.
		f, err := os.CreateTemp(filepath.Dir(path), ".linux-users-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		if err = f.Chmod(0644); err == nil {
			_, err = f.Write(append(data, '\n'))
		}
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		return os.Link(f.Name(), path)
	} else if err != nil {
		return err
	}
	return writeJSONAtomic(path, users, 0644)
}
