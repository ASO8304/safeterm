package broker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/crypto/ssh"
)

const maxIdentityFileBytes = 1 << 20

var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,127}$`)

// Principal is the identity established by SSH public-key authentication.
// Roles are loaded by the Broker and never accepted from the remote client.
type Principal struct {
	UserID     string
	Roles      []string
	BrokerType string
}

type SSHUser struct {
	UserID         string   `json:"user_id"`
	Roles          []string `json:"roles"`
	AuthorizedKeys []string `json:"authorized_keys"`
}

type SSHIdentityConfig struct {
	Users []SSHUser `json:"users"`
}

type SSHAuthenticator struct {
	byFingerprint map[string]Principal
}

func LoadSSHAuthenticator(path string, knownRoles map[string]bool) (*SSHAuthenticator, error) {
	data, err := readSecurityFile(path, "SSH identity file", maxIdentityFileBytes, 0o022)
	if err != nil {
		return nil, err
	}
	var config SSHIdentityConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("broker: decode SSH identity file: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("broker: SSH identity file has trailing data")
	}
	return NewSSHAuthenticator(config, knownRoles)
}

func NewSSHAuthenticator(config SSHIdentityConfig, knownRoles map[string]bool) (*SSHAuthenticator, error) {
	if len(config.Users) == 0 || len(knownRoles) == 0 {
		return nil, fmt.Errorf("broker: at least one SSH user and known role are required")
	}
	result := &SSHAuthenticator{byFingerprint: map[string]Principal{}}
	seenUsers := map[string]bool{}
	for _, user := range config.Users {
		if !identityPattern.MatchString(user.UserID) || seenUsers[user.UserID] {
			return nil, fmt.Errorf("broker: invalid or duplicate SSH user %q", user.UserID)
		}
		seenUsers[user.UserID] = true
		if len(user.Roles) == 0 || len(user.AuthorizedKeys) == 0 {
			return nil, fmt.Errorf("broker: SSH user %q requires roles and authorized keys", user.UserID)
		}
		roleSet := map[string]bool{}
		roles := make([]string, 0, len(user.Roles))
		for _, role := range user.Roles {
			if !knownRoles[role] || roleSet[role] {
				return nil, fmt.Errorf("broker: SSH user %q has unknown or duplicate role %q", user.UserID, role)
			}
			roleSet[role] = true
			roles = append(roles, role)
		}
		sort.Strings(roles)
		for _, encoded := range user.AuthorizedKeys {
			key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(encoded)))
			if err != nil || len(options) != 0 || len(bytes.TrimSpace(rest)) != 0 {
				return nil, fmt.Errorf("broker: SSH user %q has an invalid authorized key", user.UserID)
			}
			fingerprint := ssh.FingerprintSHA256(key)
			if _, exists := result.byFingerprint[fingerprint]; exists {
				return nil, fmt.Errorf("broker: authorized key %q is assigned more than once", fingerprint)
			}
			result.byFingerprint[fingerprint] = Principal{UserID: user.UserID, Roles: append([]string(nil), roles...), BrokerType: "ssh"}
		}
	}
	return result, nil
}

func (a *SSHAuthenticator) Authenticate(metadata ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	if a == nil || metadata == nil || key == nil {
		return nil, fmt.Errorf("broker: public-key authentication failed")
	}
	principal, ok := a.byFingerprint[ssh.FingerprintSHA256(key)]
	if !ok || metadata.User() != principal.UserID {
		return nil, fmt.Errorf("broker: public-key authentication failed")
	}
	return &ssh.Permissions{Extensions: map[string]string{
		"safeterm.user_id": principal.UserID,
		"safeterm.roles":   strings.Join(principal.Roles, "\x1f"),
	}}, nil
}

func principalFromPermissions(permissions *ssh.Permissions) (Principal, error) {
	if permissions == nil || permissions.Extensions == nil {
		return Principal{}, fmt.Errorf("broker: authenticated principal is missing")
	}
	principal := Principal{UserID: permissions.Extensions["safeterm.user_id"], BrokerType: "ssh"}
	if encoded := permissions.Extensions["safeterm.roles"]; encoded != "" {
		principal.Roles = strings.Split(encoded, "\x1f")
	}
	if !identityPattern.MatchString(principal.UserID) || len(principal.Roles) == 0 {
		return Principal{}, fmt.Errorf("broker: authenticated principal is invalid")
	}
	return principal, nil
}
