package main

import (
	"bytes"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/identity"
)

func TestLinuxConsoleMappingRequiresNoInternalPasswordAndCanRemoveMissingAccount(t *testing.T) {
	account, err := user.Current()
	if err != nil || account.Uid == "0" {
		t.Skip("requires individual non-root test user")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "access/console"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Roles: []config.RoleDefinition{{Name: "viewer"}, {Name: "netops"}}}
	var out bytes.Buffer
	o := userOptions{id: "local-admin", osUser: account.Username, roles: "viewer"}
	if err := linuxConsoleUser(dir, "add", o, cfg, &out); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "access/console/linux-users.json")
	u, err := identity.LoadLocalUsers(path, map[string]bool{"viewer": true, "netops": true})
	if err != nil || len(u.Users) != 1 || u.Users[0].OSUser != account.Username {
		t.Fatal(u, err)
	}
	o.roles = "unknown"
	if err := linuxConsoleUser(dir, "auth", o, cfg, &out); err == nil {
		t.Fatal("unknown role accepted")
	}
	o.roles = "netops"
	if err := linuxConsoleUser(dir, "auth", o, cfg, &out); err != nil {
		t.Fatal(err)
	}
	// Stale mappings must remain removable after Linux deletes the account.
	u.Users[0].OSUser = "deleted-safeconsole-user"
	if err := validateAndWriteLocalUsers(path, u, map[string]bool{"viewer": true, "netops": true}); err != nil {
		t.Fatal(err)
	}
	o.osUser = u.Users[0].OSUser
	if err := linuxConsoleUser(dir, "del", o, cfg, &out); err != nil {
		t.Fatal(err)
	}
	u, err = identity.LoadLocalUsers(path, nil)
	if err != nil || len(u.Users) != 0 {
		t.Fatal(u, err)
	}
}
