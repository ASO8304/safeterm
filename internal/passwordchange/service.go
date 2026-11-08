// Package passwordchange implements the narrow, audited self-service password
// boundary. Clients cannot select files, Linux accounts, roles, or commands.
package passwordchange

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"safeterm/internal/audit"
	"safeterm/internal/broker"
	"safeterm/internal/contract"
	"safeterm/internal/identity"
	"safeterm/internal/networkmanager"
)

const Socket = "/run/safeterm-password-manager/manager.sock"

var ErrDenied = errors.New("current password is incorrect or password change is not allowed")
var ErrChanged = errors.New("password changed; sign in again to continue")
var validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,127}$`)

type Peer struct {
	UID uint32
	PID int32
}
type Request struct {
	Version    int    `json:"version"`
	BrokerType string `json:"broker_type"`
	UserID     string `json:"user_id"`
	Session    string `json:"session,omitempty"`
	Current    []byte `json:"current"`
	New        []byte `json:"new"`
}
type Account struct {
	Name     string
	UID, GID uint32
}
type Options struct {
	Root               string
	SSHUID, ConsoleUID uint32
	Roles              map[string]bool
	Verify             func(context.Context, string, Peer) (identity.Session, error)
	LinuxChange        func(context.Context, Account, []byte, []byte) error
	Lookup             func(string) (Account, error)
	Audit              audit.Sink
}
type Service struct {
	mu       sync.Mutex
	opts     Options
	attempts map[string]int
	window   time.Time
	total    int
}

func LookupAccount(name string) (Account, error) {
	if name == "root" || name == "safeterm" || strings.HasPrefix(name, "safeterm-") {
		return Account{}, ErrDenied
	}
	u, err := user.Lookup(name)
	if err != nil {
		return Account{}, ErrDenied
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil || uid == 0 {
		return Account{}, ErrDenied
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return Account{}, ErrDenied
	}
	return Account{Name: u.Username, UID: uint32(uid), GID: uint32(gid)}, nil
}
func New(opts Options) (*Service, error) {
	if opts.Root == "" || len(opts.Roles) == 0 || opts.Verify == nil || opts.Audit == nil || opts.SSHUID == opts.ConsoleUID || opts.SSHUID == 0 || opts.ConsoleUID == 0 {
		return nil, fmt.Errorf("password: invalid service settings")
	}
	if opts.Lookup == nil {
		opts.Lookup = LookupAccount
	}
	if opts.LinuxChange == nil {
		opts.LinuxChange = ChangeLinuxPassword
	}
	return &Service{opts: opts, attempts: map[string]int{}, window: time.Now()}, nil
}
func (s *Service) record(ctx context.Context, r Request, outcome string) error {
	switch outcome {
	case "password-change-authorized":
		outcome = "apply-authorized"
	case "password-change-denied":
		outcome = "apply-rejected"
	case "password-changed":
		outcome = "applied"
	}
	req, err := contract.NewExecutionRequest("password-change", nil, contract.Metadata{BrokerType: r.BrokerType, UserID: r.UserID})
	if err != nil {
		return err
	}
	event := audit.NewEvent(req, "password-v1", audit.ComponentConfigManager, "file-writer", outcome, nil)
	event.ResourceID = "self-password:" + r.BrokerType
	return s.opts.Audit.Record(ctx, event)
}

func (s *Service) Change(ctx context.Context, p Peer, r Request) error {
	if p.PID <= 0 || r.Version != 1 || !validID.MatchString(r.UserID) || len(r.Current) == 0 || len(r.Current) > 512 || bytes.ContainsAny(r.Current, "\x00\r\n") {
		return ErrDenied
	}
	if err := broker.ValidatePasswordComplexity(r.New); err != nil {
		return err
	}
	if bytes.Equal(r.Current, r.New) {
		return fmt.Errorf("new password must differ from the current password")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if time.Since(s.window) >= time.Minute {
		s.window = time.Now()
		s.total = 0
		s.attempts = map[string]int{}
	}
	key := r.BrokerType + ":" + r.UserID
	if s.total >= 30 || s.attempts[key] >= 5 {
		return fmt.Errorf("too many password-change attempts; try again in a minute")
	}
	s.total++
	s.attempts[key]++
	if r.BrokerType == "ssh" {
		if p.UID != s.opts.SSHUID || r.Session != "" {
			return ErrDenied
		}
		return s.changeStored(ctx, r, true)
	}
	if r.BrokerType != "console" || p.UID == s.opts.SSHUID {
		return ErrDenied
	}
	session, err := s.opts.Verify(ctx, r.Session, p)
	if err != nil || session.UserID != r.UserID || !session.ExpiresAt.After(time.Now()) {
		return ErrDenied
	}
	if p.UID == s.opts.ConsoleUID {
		return s.changeStored(ctx, r, false)
	}
	users, err := identity.LoadLocalUsers(filepath.Join(s.opts.Root, "etc/safeterm/access/console/linux-users.json"), s.opts.Roles)
	if err != nil {
		return ErrDenied
	}
	for _, u := range users.Users {
		if u.Disabled || u.UserID != r.UserID {
			continue
		}
		a, err := s.opts.Lookup(u.OSUser)
		if err != nil || a.UID != p.UID {
			continue
		}
		if err := s.record(ctx, r, "password-change-authorized"); err != nil {
			return fmt.Errorf("password change denied: audit unavailable")
		}
		if err := s.opts.LinuxChange(ctx, a, r.Current, r.New); err != nil {
			_ = s.record(ctx, r, "password-change-denied")
			return err
		}
		if s.record(ctx, r, "password-changed") != nil {
			return ErrChanged
		}
		return nil
	}
	return ErrDenied
}

func (s *Service) changeStored(ctx context.Context, r Request, ssh bool) error {
	relative := "etc/safeterm/access/console/users.json"
	if ssh {
		relative = "etc/safeterm/access/ssh/users.json"
	}
	f, err := readCredential(s.opts.Root, relative)
	if err != nil {
		return fmt.Errorf("password file is unavailable or unsafe")
	}
	defer clear(f.data)
	var value any
	var old string
	var update func(string)
	if ssh {
		var users broker.SSHIdentityConfig
		if networkmanager.DecodeJSON(f.data, &users) != nil {
			return ErrDenied
		}
		if _, err := broker.NewSSHAuthenticator(users, s.opts.Roles); err != nil {
			return ErrDenied
		}
		for i := range users.Users {
			if users.Users[i].UserID == r.UserID {
				old = users.Users[i].PasswordHash
				index := i
				update = func(hash string) { users.Users[index].PasswordHash = hash }
				break
			}
		}
		value = &users
	} else {
		users, err := identity.ParseUsers(f.data, s.opts.Roles)
		if err != nil {
			return ErrDenied
		}
		for i := range users.Users {
			if users.Users[i].UserID == r.UserID && !users.Users[i].Disabled {
				old = users.Users[i].PasswordHash
				index := i
				update = func(hash string) { users.Users[index].PasswordHash = hash }
				break
			}
		}
		value = &users
	}
	if old == "" || update == nil || bcrypt.CompareHashAndPassword([]byte(old), r.Current) != nil {
		_ = s.record(ctx, r, "password-change-denied")
		return ErrDenied
	}
	hash, err := bcrypt.GenerateFromPassword(r.New, broker.PasswordHashCost)
	if err != nil {
		return ErrDenied
	}
	defer clear(hash)
	update(string(hash))
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ErrDenied
	}
	defer clear(data)
	if s.record(ctx, r, "password-change-authorized") != nil {
		return fmt.Errorf("password change denied: audit unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := replaceCredential(s.opts.Root, relative, f, append(data, '\n')); err != nil {
		return err
	}
	if s.record(ctx, r, "password-changed") != nil {
		return ErrChanged
	}
	return nil
}
