// Package identity authenticates internal Console users and issues opaque,
// process-bound sessions. The shared OS account never reads password hashes.
package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"safeterm/internal/audit"
	"safeterm/internal/broker"
	"safeterm/internal/contract"
)

const SocketPath = "/run/safeterm-identity/identity.sock"
const UserName = "safeterm-identity"

var ErrDenied = errors.New("identity: authentication or session denied")

type Peer struct {
	UID uint32
	PID int32
}
type Session struct {
	Token     string    `json:"token,omitempty"`
	UserID    string    `json:"user_id"`
	Roles     []string  `json:"roles"`
	ExpiresAt time.Time `json:"expires_at"`
}
type grant struct {
	session    Session
	peer       Peer
	credential [32]byte
}
type attempt struct {
	start time.Time
	count int
}
type Service struct {
	mu        sync.Mutex
	usersPath string
	roles     map[string]bool
	ttl       time.Duration
	now       func() time.Time
	sessions  map[[32]byte]grant
	attempts  map[string]attempt
	global    attempt
	slots     chan struct{}
	dummy     []byte
	sink      audit.Sink
	version   string
}

// ParseUsers accepts old OS-user mappings for migration only; entries without
// passwords are disabled. No OS account mapping can authenticate a session.
func ParseUsers(data []byte, known map[string]bool) (broker.ConsoleIdentityConfig, error) {
	var users broker.ConsoleIdentityConfig
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&users) != nil {
		return users, errors.New("identity: invalid console users file")
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF || len(users.Users) == 0 || len(users.Users) > 4096 {
		return users, errors.New("identity: invalid console users file")
	}
	seen := map[string]bool{}
	for i := range users.Users {
		u := &users.Users[i]
		if !validID(u.UserID) || seen[u.UserID] || len(u.Roles) == 0 {
			return users, errors.New("identity: invalid or duplicate console user")
		}
		seen[u.UserID] = true
		set := map[string]bool{}
		for _, r := range u.Roles {
			if !known[r] || set[r] {
				return users, errors.New("identity: invalid console user roles")
			}
			set[r] = true
		}
		if u.PasswordHash != "" {
			if broker.ValidatePasswordHash(u.PasswordHash) != nil {
				return users, errors.New("identity: invalid password hash")
			}
		}
		if u.PasswordHash == "" {
			u.Disabled = true
		}
		sort.Strings(u.Roles)
	}
	return users, nil
}
func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for i, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if i > 0 && (c == '_' || c == '-' || c == '.' || c == '@') {
			continue
		}
		return false
	}
	return true
}
func New(path string, roles map[string]bool, ttl time.Duration, sink audit.Sink, version string) (*Service, error) {
	if path == "" || len(roles) == 0 || ttl < time.Minute || ttl > 12*time.Hour || sink == nil {
		return nil, errors.New("identity: invalid service settings")
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("RandomDummyPasswordForUnknownUser"), broker.PasswordHashCost)
	if err != nil {
		return nil, err
	}
	copyRoles := map[string]bool{}
	for r, v := range roles {
		copyRoles[r] = v
	}
	s := &Service{usersPath: path, roles: copyRoles, ttl: ttl, now: time.Now, sessions: map[[32]byte]grant{}, attempts: map[string]attempt{}, slots: make(chan struct{}, 4), dummy: dummy, sink: sink, version: version}
	_, err = s.load()
	return s, err
}
func (s *Service) load() (broker.ConsoleIdentityConfig, error) {
	// Non-symlink and bounded reads use the same security-file
	// boundary as the Brokers. The identity daemon has read access only.
	data, err := broker.ReadConsoleUsersData(s.usersPath)
	if err != nil {
		return broker.ConsoleIdentityConfig{}, err
	}
	return ParseUsers(data, s.roles)
}
func credential(u broker.ConsoleIdentity) [32]byte {
	u.OSUser = ""
	b, _ := json.Marshal(u)
	return sha256.Sum256(b)
}
func (s *Service) record(ctx context.Context, user, outcome string) error {
	if !validID(user) {
		user = "unknown"
	}
	req, err := contract.NewExecutionRequest("console-login", nil, contract.Metadata{BrokerType: "console", UserID: user})
	if err != nil {
		return err
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	return s.sink.Record(auditCtx, audit.NewEvent(req, s.version, audit.ComponentIdentity, "", outcome, nil))
}
func (s *Service) Login(ctx context.Context, p Peer, user string, password []byte) (Session, error) {
	deny := func() (Session, error) { _ = s.record(ctx, user, "authentication-failed"); return Session{}, ErrDenied }
	if p.PID <= 0 || !validID(user) || len(password) == 0 || len(password) > 72 {
		return deny()
	}
	now := s.now().UTC()
	s.mu.Lock()
	if now.Sub(s.global.start) >= time.Minute {
		s.global = attempt{start: now}
		s.attempts = map[string]attempt{}
	}
	a := s.attempts[user]
	if now.Sub(a.start) >= time.Minute {
		a = attempt{start: now}
	}
	limited := s.global.count >= 30 || a.count >= 5 || len(s.attempts) >= 1024
	if !limited {
		s.global.count++
		a.count++
		s.attempts[user] = a
	}
	s.mu.Unlock()
	if limited {
		return deny()
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return deny()
	}
	users, err := s.load()
	if err != nil {
		return deny()
	}
	var selected *broker.ConsoleIdentity
	hash := s.dummy
	for i := range users.Users {
		u := &users.Users[i]
		if u.UserID == user && !u.Disabled {
			selected = u
			hash = []byte(u.PasswordHash)
			break
		}
	}
	err = bcrypt.CompareHashAndPassword(hash, password)
	if err != nil || selected == nil {
		return deny()
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		return Session{}, ErrDenied
	}
	session := Session{Token: hex.EncodeToString(random[:]), UserID: selected.UserID, Roles: append([]string(nil), selected.Roles...), ExpiresAt: now.Add(s.ttl)}
	if err = s.record(ctx, user, "authenticated"); err != nil {
		return Session{}, ErrDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, g := range s.sessions {
		if !now.Before(g.session.ExpiresAt) || g.peer == p {
			delete(s.sessions, key)
		}
	}
	if len(s.sessions) >= 1024 {
		return Session{}, ErrDenied
	}
	key := sha256.Sum256([]byte(session.Token))
	stored := session
	stored.Token = ""
	s.sessions[key] = grant{stored, p, credential(*selected)}
	return session, nil
}
func (s *Service) Verify(token string, p Peer) (Session, error) {
	if len(token) != 64 || p.PID <= 0 {
		return Session{}, ErrDenied
	}
	key := sha256.Sum256([]byte(token))
	s.mu.Lock()
	g, ok := s.sessions[key]
	s.mu.Unlock()
	if !ok || g.peer != p || !s.now().UTC().Before(g.session.ExpiresAt) {
		return Session{}, ErrDenied
	}
	users, err := s.load()
	if err != nil {
		return Session{}, ErrDenied
	}
	for _, u := range users.Users {
		if u.UserID == g.session.UserID && !u.Disabled && credential(u) == g.credential {
			result := g.session
			result.Roles = append([]string(nil), result.Roles...)
			return result, nil
		}
	}
	s.mu.Lock()
	delete(s.sessions, key)
	s.mu.Unlock()
	return Session{}, ErrDenied
}
func (s *Service) Logout(ctx context.Context, token string, p Peer) error {
	session, err := s.Verify(token, p)
	if err != nil {
		return ErrDenied
	}
	key := sha256.Sum256([]byte(token))
	s.mu.Lock()
	delete(s.sessions, key)
	s.mu.Unlock()
	return s.record(ctx, session.UserID, "logged-out")
}
