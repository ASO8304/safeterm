package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

const LocalUsersPath = "/etc/safeterm/access/console/linux-users.json"

// Linux mappings contain no secrets and are readable by the boundary services.
// Only the Linux login authenticates the operator; roles still come from root.
type LocalUser struct {
	OSUser   string   `json:"os_user"`
	UserID   string   `json:"user_id"`
	Roles    []string `json:"roles"`
	Disabled bool     `json:"disabled,omitempty"`
}
type LocalUsers struct {
	Users []LocalUser `json:"users"`
}

func ParseLocalUsers(data []byte, known map[string]bool) (LocalUsers, error) {
	var result LocalUsers
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&result); err != nil {
		return result, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF || len(result.Users) > 4096 {
		return result, ErrDenied
	}
	ids, accounts := map[string]bool{}, map[string]bool{}
	for _, u := range result.Users {
		if !validID(u.UserID) || !validID(u.OSUser) || reservedLocalUser(u.OSUser) || ids[u.UserID] || accounts[u.OSUser] || len(u.Roles) == 0 {
			return result, fmt.Errorf("identity: invalid or duplicate Linux Console mapping")
		}
		ids[u.UserID], accounts[u.OSUser] = true, true
		seen := map[string]bool{}
		for _, role := range u.Roles {
			if !validID(role) || seen[role] || known != nil && !known[role] {
				return result, fmt.Errorf("identity: invalid Linux Console role")
			}
			seen[role] = true
		}
	}
	return result, nil
}

func reservedLocalUser(name string) bool {
	return name == "root" || name == "safeterm" || strings.HasPrefix(name, "safeterm-")
}

func localUID(name string) (uint32, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil || n == 0 || reservedLocalUser(u.Username) {
		return 0, ErrDenied
	}
	return uint32(n), nil
}

func LoadLocalUsers(path string, known map[string]bool) (LocalUsers, error) {
	data, err := readLocalUsers(path)
	if os.IsNotExist(err) {
		return LocalUsers{}, nil
	}
	if err != nil {
		return LocalUsers{}, err
	}
	return ParseLocalUsers(data, known)
}

// Resolve only explicitly enabled existing Linux accounts. Missing accounts do
// not break startup or unrelated mappings; login fails closed for those entries.
func LocalUIDs(path string) ([]uint32, error) {
	users, err := LoadLocalUsers(path, nil)
	if err != nil {
		return nil, err
	}
	seen := map[uint32]bool{}
	var result []uint32
	for _, u := range users.Users {
		if u.Disabled {
			continue
		}
		uid, err := localUID(u.OSUser)
		if err != nil {
			continue
		}
		if seen[uid] {
			return nil, fmt.Errorf("identity: Linux Console accounts must have distinct UIDs")
		}
		seen[uid] = true
		result = append(result, uid)
	}
	return result, nil
}

func localCredential(u LocalUser) [32]byte { data, _ := json.Marshal(u); return sha256.Sum256(data) }

func (s *Service) LoginLocal(ctx context.Context, p Peer) (Session, error) {
	if p.PID <= 0 {
		return Session{}, ErrDenied
	}
	users, err := LoadLocalUsers(filepath.Join(filepath.Dir(s.usersPath), "linux-users.json"), s.roles)
	if err != nil {
		return Session{}, ErrDenied
	}
	var selected *LocalUser
	for i, u := range users.Users {
		uid, err := localUID(u.OSUser)
		if !u.Disabled && err == nil && uid == p.UID {
			if selected != nil {
				return Session{}, ErrDenied
			}
			selected = &users.Users[i]
		}
	}
	if selected == nil {
		return Session{}, ErrDenied
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Session{}, ErrDenied
	}
	now := s.now().UTC()
	session := Session{Token: hex.EncodeToString(random[:]), UserID: selected.UserID, Roles: append([]string(nil), selected.Roles...), ExpiresAt: now.Add(s.ttl)}
	if s.record(ctx, selected.UserID, "authenticated") != nil {
		return Session{}, ErrDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, g := range s.sessions {
		if !now.Before(g.session.ExpiresAt) || g.peer == p {
			delete(s.sessions, k)
		}
	}
	if len(s.sessions) >= 1024 {
		return Session{}, ErrDenied
	}
	stored := session
	stored.Token = ""
	s.sessions[sha256.Sum256([]byte(session.Token))] = grant{session: stored, peer: p, credential: localCredential(*selected), local: true}
	return session, nil
}

func (s *Service) verifyLocal(g grant) bool {
	users, err := LoadLocalUsers(filepath.Join(filepath.Dir(s.usersPath), "linux-users.json"), s.roles)
	if err != nil {
		return false
	}
	for _, u := range users.Users {
		uid, err := localUID(u.OSUser)
		if err == nil && uid == g.peer.UID && !u.Disabled && u.UserID == g.session.UserID && localCredential(u) == g.credential {
			return true
		}
	}
	return false
}
