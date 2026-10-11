package broker

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"io"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/crypto/ssh"
)

const maxIdentityFileBytes = 1 << 20

var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,127}$`)

// Principal is the identity established by SSH authentication.
// Roles are loaded by the Broker and never accepted from the remote client.
type Principal struct {
	UserID     string
	Roles      []string
	BrokerType string
}

type SSHUser struct {
	UserID         string   `json:"user_id"`
	Roles          []string `json:"roles"`
	AuthorizedKeys []string `json:"authorized_keys,omitempty"`
	PasswordHash   string   `json:"password_hash,omitempty"`
}

type SSHIdentityConfig struct {
	Users []SSHUser `json:"users"`
}

type SSHAuthenticator struct {
	byFingerprint map[string]Principal
	byPassword    map[string]passwordIdentity
	passwordSlots chan struct{}
	dummyHash     []byte
}

func LoadSSHAuthenticator(path string, knownRoles map[string]bool) (*SSHAuthenticator, error) {
	data, err := readSecurityFile(path, "SSH identity file", maxIdentityFileBytes, 0o027)
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
	result := &SSHAuthenticator{byFingerprint: map[string]Principal{}, byPassword: map[string]passwordIdentity{}, passwordSlots: make(chan struct{}, 4)}
	seenUsers := map[string]bool{}
	for _, user := range config.Users {
		if !identityPattern.MatchString(user.UserID) || seenUsers[user.UserID] {
			return nil, fmt.Errorf("broker: invalid or duplicate SSH user %q", user.UserID)
		}
		seenUsers[user.UserID] = true
		if len(user.Roles) == 0 || (len(user.AuthorizedKeys) == 0 && user.PasswordHash == "") {
			return nil, fmt.Errorf("broker: SSH user %q requires roles and a password hash or authorized keys", user.UserID)
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
		if user.PasswordHash != "" {
			if err := ValidatePasswordHash(user.PasswordHash); err != nil {
				return nil, fmt.Errorf("broker: SSH user %q has an invalid password hash", user.UserID)
			}
			result.byPassword[user.UserID] = passwordIdentity{principal: Principal{UserID: user.UserID, Roles: append([]string(nil), roles...), BrokerType: "ssh"}, hash: []byte(user.PasswordHash)}
		}
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
	if len(result.byPassword) > 0 {
		dummy := make([]byte, 32)
		if _, err := rand.Read(dummy); err != nil {
			return nil, fmt.Errorf("broker: initialize password authentication")
		}
		var err error
		result.dummyHash, err = bcrypt.GenerateFromPassword(dummy, PasswordHashCost)
		clear(dummy)
		if err != nil {
			return nil, fmt.Errorf("broker: initialize password authentication")
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

// PasswordHashCost is the work factor used for newly generated salted bcrypt hashes.
const PasswordHashCost = 12

type passwordIdentity struct {
	principal Principal
	hash      []byte
}

var passwordHashPattern = regexp.MustCompile(`^\$2[ab]\$(10|11|12|13|14)\$[./A-Za-z0-9]{53}$`)

// ValidatePasswordComplexity enforces the interactive password policy used by
// the administrator tools. Bcrypt still limits the encoded input to 72 bytes.
func ValidatePasswordComplexity(password []byte) error {
	if len(password) < 8 || len(password) > 72 {
		return fmt.Errorf("password must contain 8 through 72 bytes")
	}
	if !regexp.MustCompile(`[A-Z]`).Match(password) ||
		!regexp.MustCompile(`[a-z]`).Match(password) ||
		!regexp.MustCompile(`[0-9]`).Match(password) {
		return fmt.Errorf("password must contain uppercase, lowercase, and numeric characters")
	}
	for _, value := range string(password) {
		if value < 0x20 || value == 0x7f {
			return fmt.Errorf("password must not contain control characters")
		}
	}
	return nil
}

// ValidatePasswordHash rejects plaintext, malformed hashes and unbounded work factors.
func ValidatePasswordHash(encoded string) error {
	if !passwordHashPattern.MatchString(encoded) {
		return fmt.Errorf("broker: expected bcrypt hash with cost 10 through 14")
	}
	if _, err := bcrypt.Cost([]byte(encoded)); err != nil {
		return fmt.Errorf("broker: invalid bcrypt hash")
	}
	return nil
}

// AuthenticatePassword uses only the configured SafeTerm identity, never OS passwords.
func (a *SSHAuthenticator) AuthenticatePassword(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	failed := fmt.Errorf("broker: password authentication failed")
	if a == nil || metadata == nil || len(password) == 0 || len(password) > 72 {
		return nil, failed
	}
	// Bound expensive password work independently of established SSH sessions.
	select {
	case a.passwordSlots <- struct{}{}:
	default:
		return nil, failed
	}
	defer func() { <-a.passwordSlots }()
	identity, exists := a.byPassword[metadata.User()]
	hash := a.dummyHash
	if exists {
		hash = identity.hash
	}
	err := bcrypt.CompareHashAndPassword(hash, password)
	if err != nil || !exists {
		return nil, failed
	}
	return &ssh.Permissions{Extensions: map[string]string{
		"safeterm.user_id": identity.principal.UserID,
		"safeterm.roles":   strings.Join(identity.principal.Roles, "\x1f"),
	}}, nil
}
