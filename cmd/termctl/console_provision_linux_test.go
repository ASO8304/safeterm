//go:build linux

package main

import (
	"bytes"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/identity"
)

type provisionFixture struct {
	calls         []string
	checks        int
	failAt        string
	passwordBytes []byte
	passwordSet   bool
}

func (h *provisionFixture) available(name string) error {
	h.checks++
	if h.failAt == "existing" || h.failAt == "race" && h.checks == 2 {
		return errors.New("Linux user already exists; nothing changed")
	}
	return nil
}
func (h *provisionFixture) password() ([]byte, error) {
	h.calls = append(h.calls, "password")
	if h.failAt == "password" {
		return nil, errors.New("cancelled")
	}
	h.passwordBytes = []byte("ConsolePassword42")
	return h.passwordBytes, nil
}
func (h *provisionFixture) lock() (func(), error) {
	h.calls = append(h.calls, "lock")
	if h.failAt == "lock" {
		return nil, errors.New("network trial active")
	}
	return func() { h.calls = append(h.calls, "unlock") }, nil
}
func (h *provisionFixture) create(name string) (string, error) {
	h.calls = append(h.calls, "create:"+name)
	if h.failAt == "create" {
		return "", errors.New("useradd failed")
	}
	return "2000", nil
}
func (h *provisionFixture) reload() error {
	h.calls = append(h.calls, "reload")
	if h.failAt == "reload" {
		return errors.New("reload failed")
	}
	return nil
}
func (h *provisionFixture) activate(name, uid string, p []byte) error {
	h.calls = append(h.calls, "activate:"+name+":"+uid)
	if string(p) != "ConsolePassword42" {
		return errors.New("wrong password")
	}
	h.passwordSet = true
	if h.failAt == "activate" {
		return errors.New("PAM completion failed")
	}
	return nil
}
func (h *provisionFixture) disable(name, uid string) error {
	h.calls = append(h.calls, "disable:"+name+":"+uid)
	h.passwordSet = false
	return nil
}
func provisionTest(t *testing.T) (string, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "access/console"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "access/console/linux-users.json"), []byte(`{"users":[{"os_user":"existing-operator","user_id":"existing","roles":["viewer"]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	return dir, &config.Config{Roles: []config.RoleDefinition{{Name: "viewer"}, {Name: "netops"}}}
}
func TestConsoleProvisionCreatesOnlyUnusedAccountAndUnlocksLast(t *testing.T) {
	dir, cfg := provisionTest(t)
	h := &provisionFixture{}
	var out bytes.Buffer
	if err := provisionConsole(dir, userOptions{osUser: "new-operator", roles: "viewer,netops"}, cfg, h, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(h.calls, []string{"password", "lock", "create:new-operator", "reload", "activate:new-operator:2000", "unlock"}) || !h.passwordSet {
		t.Fatal(h.calls)
	}
	users, err := identity.LoadLocalUsers(filepath.Join(dir, "access/console/linux-users.json"), nil)
	if err != nil || len(users.Users) != 2 || users.Users[0].OSUser != "existing-operator" || users.Users[1].UserID != "new-operator" {
		t.Fatal(users, err)
	}
	if strings.Contains(out.String(), "ConsolePassword42") {
		t.Fatal("password in output")
	}
	for _, b := range h.passwordBytes {
		if b != 0 {
			t.Fatal("password buffer retained")
		}
	}
}
func TestConsoleProvisionRefusesExistingAccountsAndInvalidPlansWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		failure string
		o       userOptions
	}{
		{"existing", userOptions{osUser: "new-operator", roles: "viewer"}},
		{"race", userOptions{osUser: "new-operator", roles: "viewer"}},
		{"password", userOptions{osUser: "new-operator", roles: "viewer"}},
		{"lock", userOptions{osUser: "new-operator", roles: "viewer"}},
		{"", userOptions{osUser: "--bad", roles: "viewer"}},
		{"", userOptions{osUser: "root", roles: "viewer"}},
		{"", userOptions{osUser: "new-operator", roles: "unknown"}},
		{"", userOptions{osUser: "new-operator", roles: "viewer", hashFile: "/tmp/hash"}},
		{"", userOptions{osUser: "new-operator", roles: "viewer", id: "existing"}},
	} {
		t.Run(tc.failure+tc.o.osUser+tc.o.roles+tc.o.id, func(t *testing.T) {
			dir, cfg := provisionTest(t)
			path := filepath.Join(dir, "access/console/linux-users.json")
			before, _ := os.ReadFile(path)
			h := &provisionFixture{failAt: tc.failure}
			if err := provisionConsole(dir, tc.o, cfg, h, &bytes.Buffer{}); err == nil {
				t.Fatal("invalid provisioning accepted")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("existing mappings changed")
			}
			for _, call := range h.calls {
				if strings.HasPrefix(call, "create:") || strings.HasPrefix(call, "disable:") || strings.HasPrefix(call, "activate:") || call == "reload" {
					t.Fatal("rejection mutated Linux/service state", h.calls)
				}
			}
		})
	}
}
func TestConsoleProvisionFailureRemovesOnlyNewMappingAndLocksNewAccount(t *testing.T) {
	for _, failure := range []string{"reload", "activate"} {
		t.Run(failure, func(t *testing.T) {
			dir, cfg := provisionTest(t)
			h := &provisionFixture{failAt: failure}
			err := provisionConsole(dir, userOptions{osUser: "new-operator", roles: "viewer"}, cfg, h, &bytes.Buffer{})
			if err == nil || h.passwordSet || !strings.Contains(err.Error(), "password-locked") {
				t.Fatal(err, h.calls)
			}
			users, e := identity.LoadLocalUsers(filepath.Join(dir, "access/console/linux-users.json"), nil)
			if e != nil || len(users.Users) != 1 || users.Users[0].OSUser != "existing-operator" {
				t.Fatal(users, e)
			}
		})
	}
}
func TestConsoleSystemHostRefusesCurrentLinuxUserBeforeAnyChange(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	err = (consoleSystemHost{}).available(u.Username)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatal(err)
	}
}
