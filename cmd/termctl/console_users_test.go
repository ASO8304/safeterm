package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/identity"
	"testing"
)

func TestConsoleUserDoesNotRequireOrCreateAnOSAccount(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "access/console"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "access/console", "users.json")
	if err := os.WriteFile(path, []byte(`{"users":[{"os_user":"safeterm","user_id":"legacy","roles":["viewer"]}]}`), 0640); err != nil {
		t.Fatal(err)
	}
	hashFile := filepath.Join(dir, "password.hash")
	hash := "$2a$12$C6UzMDM.H6dfI/f/IKcEe.7W8QbZ0pS8Qw6tYfYfHjS2K7S5m8x2K"
	if err := os.WriteFile(hashFile, []byte(hash), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Roles: []config.RoleDefinition{{Name: "viewer"}}}
	var out bytes.Buffer
	options := userOptions{id: "internal-user-with-no-linux-account", roles: "viewer", hashFile: hashFile}
	if err := upsertConsoleUser(dir, false, options, cfg, &out, false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	users, err := identity.ParseUsers(data, map[string]bool{"viewer": true})
	if err != nil {
		t.Fatal(err)
	}
	added := users.Users[1]
	if added.OSUser != "" || added.UserID != options.id || added.PasswordHash != hash || added.Disabled {
		t.Fatalf("unexpected identity: id=%s OSUser=%s disabled=%t", added.UserID, added.OSUser, added.Disabled)
	}
	if !users.Users[0].Disabled {
		t.Fatal("legacy OS mapping silently enabled")
	}
	if err = upsertConsoleUser(dir, false, options, cfg, &out, false); err == nil {
		t.Fatal("duplicate accepted")
	}
	users.Users[1].Disabled = true
	encoded, _ := json.Marshal(users)
	if err = os.WriteFile(path, encoded, 0640); err != nil {
		t.Fatal(err)
	}
	if err = upsertConsoleUser(dir, true, userOptions{id: options.id, roles: "viewer"}, cfg, &out, false); err != nil {
		t.Fatal(err)
	}
	disabledData, _ := os.ReadFile(path)
	disabledUsers, err := identity.ParseUsers(disabledData, map[string]bool{"viewer": true})
	if err != nil || !disabledUsers.Users[1].Disabled {
		t.Fatal("roles-only update enabled disabled account")
	}
	if err = deleteUser(dir, "console", userOptions{id: options.id}, cfg, &out); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var remaining broker.ConsoleIdentityConfig
	_ = json.Unmarshal(raw, &remaining)
	if len(remaining.Users) != 1 {
		t.Fatal("delete failed")
	}
}
