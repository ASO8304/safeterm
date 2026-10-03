package broker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// ConsoleIdentity binds an operating-system login account to a SafeTerm
// identity. Roles always come from this administrator-owned file.
type ConsoleIdentity struct {
	OSUser string   `json:"os_user"`
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
}

type ConsoleIdentityConfig struct {
	Users []ConsoleIdentity `json:"users"`
}

func LoadConsolePrincipal(path, osUser string, knownRoles map[string]bool) (Principal, error) {
	data, err := readSecurityFile(path, "console identity file", maxIdentityFileBytes, 0o022)
	if err != nil {
		return Principal{}, err
	}
	return ParseConsolePrincipal(data, osUser, knownRoles)
}

func ParseConsolePrincipal(data []byte, osUser string, knownRoles map[string]bool) (Principal, error) {
	var config ConsoleIdentityConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Principal{}, fmt.Errorf("broker: decode console identity file: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Principal{}, fmt.Errorf("broker: console identity file has trailing data")
	}
	if len(config.Users) == 0 || len(knownRoles) == 0 {
		return Principal{}, fmt.Errorf("broker: at least one console user and known role are required")
	}
	seenOS, seenIDs := map[string]bool{}, map[string]bool{}
	var selected *Principal
	for _, entry := range config.Users {
		if !identityPattern.MatchString(entry.OSUser) || !identityPattern.MatchString(entry.UserID) || seenOS[entry.OSUser] || seenIDs[entry.UserID] {
			return Principal{}, fmt.Errorf("broker: invalid or duplicate console identity %q", entry.OSUser)
		}
		seenOS[entry.OSUser], seenIDs[entry.UserID] = true, true
		if len(entry.Roles) == 0 {
			return Principal{}, fmt.Errorf("broker: console user %q requires roles", entry.OSUser)
		}
		roleSet := map[string]bool{}
		roles := make([]string, 0, len(entry.Roles))
		for _, role := range entry.Roles {
			if !knownRoles[role] || roleSet[role] {
				return Principal{}, fmt.Errorf("broker: console user %q has unknown or duplicate role %q", entry.OSUser, role)
			}
			roleSet[role] = true
			roles = append(roles, role)
		}
		sort.Strings(roles)
		if entry.OSUser == osUser {
			value := Principal{UserID: entry.UserID, Roles: roles, BrokerType: "console"}
			selected = &value
		}
	}
	if selected == nil {
		return Principal{}, fmt.Errorf("broker: OS user %q is not authorized for the console", osUser)
	}
	return *selected, nil
}
