package passwordchange

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"safeterm/internal/audit"
	"safeterm/internal/broker"
	"safeterm/internal/identity"
)

type auditLog struct {
	events []audit.Event
	fail   bool
}

func (a *auditLog) Record(_ context.Context, e audit.Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if a.fail {
		return errors.New("unavailable")
	}
	a.events = append(a.events, e)
	return nil
}
func fixture(t *testing.T) (*Service, string, *auditLog) {
	t.Helper()
	root := t.TempDir()
	log := &auditLog{}
	s, err := New(Options{Root: root, SSHUID: 1101, ConsoleUID: 1102, Roles: map[string]bool{"viewer": true}, Audit: log, Verify: func(_ context.Context, token string, p Peer) (identity.Session, error) {
		if token != "token" || p.UID != 1102 && p.UID != 1103 {
			return identity.Session{}, ErrDenied
		}
		return identity.Session{UserID: "alice", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("OldPassword42"), broker.PasswordHashCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"ssh", "console"} {
		path := filepath.Join(root, "etc/safeterm/access", kind, "users.json")
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(map[string]any{"users": []any{map[string]any{"user_id": "alice", "roles": []string{"viewer"}, "password_hash": string(hash)}, map[string]any{"user_id": "bob", "roles": []string{"viewer"}, "password_hash": string(hash)}}})
		if err = os.WriteFile(path, data, 0640); err != nil {
			t.Fatal(err)
		}
	}
	return s, root, log
}
func request(kind string) Request {
	return Request{Version: 1, BrokerType: kind, UserID: "alice", Current: []byte("OldPassword42"), New: []byte("NewPassword43")}
}
func TestStoredPasswordsRemainSeparateAndRequireCurrentPassword(t *testing.T) {
	for _, kind := range []string{"ssh", "console"} {
		t.Run(kind, func(t *testing.T) {
			s, root, log := fixture(t)
			r := request(kind)
			p := Peer{1101, 100}
			other := "console"
			if kind == "console" {
				p.UID = 1102
				r.Session = "token"
				other = "ssh"
			}
			path := filepath.Join(root, "etc/safeterm/access", kind, "users.json")
			otherPath := filepath.Join(root, "etc/safeterm/access", other, "users.json")
			before, _ := os.ReadFile(path)
			otherBefore, _ := os.ReadFile(otherPath)
			bad := r
			bad.Current = []byte("WrongPassword42")
			if !errors.Is(s.Change(context.Background(), p, bad), ErrDenied) {
				t.Fatal("wrong old password accepted")
			}
			unchanged, _ := os.ReadFile(path)
			if string(unchanged) != string(before) {
				t.Fatal("wrong password changed file")
			}
			if err := s.Change(context.Background(), p, r); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(path)
			var users broker.SSHIdentityConfig
			if json.Unmarshal(data, &users) != nil {
				t.Fatal("invalid credentials")
			}
			if bcrypt.CompareHashAndPassword([]byte(users.Users[0].PasswordHash), r.New) != nil || bcrypt.CompareHashAndPassword([]byte(users.Users[0].PasswordHash), r.Current) == nil {
				t.Fatal("password not changed")
			}
			var previous broker.SSHIdentityConfig
			_ = json.Unmarshal(before, &previous)
			if users.Users[1].PasswordHash != previous.Users[1].PasswordHash || strings.Join(users.Users[0].Roles, ",") != "viewer" {
				t.Fatal("other credentials or roles changed")
			}
			otherAfter, _ := os.ReadFile(otherPath)
			if string(otherBefore) != string(otherAfter) {
				t.Fatal("changed another login database")
			}
			info, _ := os.Stat(path)
			if info.Mode().Perm() != 0640 {
				t.Fatal("credential permissions changed")
			}
			events, _ := json.Marshal(log.events)
			if strings.Contains(string(events), string(r.Current)) || strings.Contains(string(events), string(r.New)) {
				t.Fatal("password in audit")
			}
		})
	}
}
func TestPasswordBoundaryRejectsForgeryUnsafeFilesAndAuditFailure(t *testing.T) {
	s, root, log := fixture(t)
	r := request("ssh")
	path := filepath.Join(root, "etc/safeterm/access/ssh/users.json")
	before, _ := os.ReadFile(path)
	for _, p := range []Peer{{0, 1}, {1102, 1}, {1103, 1}, {1101, 0}} {
		if s.Change(context.Background(), p, r) == nil {
			t.Fatal("untrusted peer accepted", p)
		}
	}
	console := request("console")
	console.Session = "invalid"
	if s.Change(context.Background(), Peer{1102, 1}, console) == nil {
		t.Fatal("forged Console session accepted")
	}
	log.fail = true
	if s.Change(context.Background(), Peer{1101, 1}, r) == nil {
		t.Fatal("changed without audit")
	}
	log.fail = false
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if s.Change(context.Background(), Peer{1101, 1}, r) == nil {
		t.Fatal("unsafe file accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("rejection changed credentials")
	}
}
func TestMappedConsoleCanChangeOnlyItsBoundLinuxAccount(t *testing.T) {
	s, root, _ := fixture(t)
	path := filepath.Join(root, "etc/safeterm/access/console/linux-users.json")
	data := []byte(`{"users":[{"os_user":"operator","user_id":"alice","roles":["viewer"]}]}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.opts.Lookup = func(name string) (Account, error) {
		if name != "operator" {
			t.Fatal(name)
		}
		return Account{Name: name, UID: 1103, GID: 1103}, nil
	}
	s.opts.LinuxChange = func(_ context.Context, a Account, old, next []byte) error {
		calls++
		if a.Name != "operator" || string(old) != "OldPassword42" || string(next) != "NewPassword43" {
			t.Fatal("incorrect native request")
		}
		return nil
	}
	r := request("console")
	r.Session = "token"
	if err := s.Change(context.Background(), Peer{1103, 55}, r); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
	s.opts.Lookup = func(string) (Account, error) { return Account{Name: "operator", UID: 9999}, nil }
	if s.Change(context.Background(), Peer{1103, 55}, r) == nil || calls != 1 {
		t.Fatal("unbound Linux account changed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s.Change(ctx, Peer{1103, 55}, r) == nil || calls != 1 {
		t.Fatal("expired request reached PAM")
	}
}
func TestLinuxPasswordChildRejectsUnprivilegedAndReservedAccounts(t *testing.T) {
	// Never exercise real password changes on the developer's machine.
	for _, name := range []string{"root", "safeterm", "safeterm-broker", "--help", "../root"} {
		if LinuxPasswordChild(name) == nil {
			t.Fatal("unsafe child target accepted")
		}
	}
}

func TestNativeIDsRejectRootOverflowAndUnchangedSentinels(t *testing.T) {
	for _, a := range []Account{{UID: 0, GID: 1000}, {UID: math.MaxUint32, GID: 1000}, {UID: 1000, GID: math.MaxUint32}, {UID: math.MaxInt32 + 1, GID: 1000}} {
		if _, _, err := nativeIDs(a); err == nil {
			t.Fatal("unsafe credential conversion accepted", a)
		}
	}
	uid, gid, err := nativeIDs(Account{UID: 1000, GID: 1001})
	if err != nil || uid != 1000 || gid != 1001 {
		t.Fatal(uid, gid, err)
	}
}
