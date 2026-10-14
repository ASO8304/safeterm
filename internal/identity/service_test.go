package identity

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"safeterm/internal/audit"
	"safeterm/internal/broker"
)

type recorder struct {
	mu     sync.Mutex
	events []audit.Event
	fail   bool
}

func (r *recorder) Record(_ context.Context, e audit.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("audit unavailable")
	}
	if err := e.Validate(); err != nil {
		return err
	}
	r.events = append(r.events, e)
	return nil
}
func fixture(t *testing.T) (*Service, *recorder) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("GoodPassword42"), 12)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "users.json")
	data, _ := json.Marshal(broker.ConsoleIdentityConfig{Users: []broker.ConsoleIdentity{{UserID: "alice", Roles: []string{"viewer"}, PasswordHash: string(hash)}, {UserID: "legacy", OSUser: "safeterm", Roles: []string{"viewer"}}}})
	if err = os.WriteFile(p, data, 0640); err != nil {
		t.Fatal(err)
	}
	sink := &recorder{}
	s, err := New(p, map[string]bool{"viewer": true, "netops": true}, time.Minute, sink, "test")
	if err != nil {
		t.Fatal(err)
	}
	return s, sink
}
func TestSessionsAreProcessBoundExpireAndLogout(t *testing.T) {
	s, sink := fixture(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	peer := Peer{1000, 123}
	login, err := s.Login(context.Background(), peer, "alice", []byte("GoodPassword42"))
	if err != nil {
		t.Fatal(err)
	}
	if len(login.Token) != 64 || login.UserID != "alice" {
		t.Fatal("invalid login result")
	}
	if _, err = s.Verify(login.Token, peer); err != nil {
		t.Fatal(err)
	}
	for _, other := range []Peer{{1000, 124}, {1001, 123}} {
		if _, err = s.Verify(login.Token, other); err == nil {
			t.Fatal("session accepted from another process")
		}
	}
	if _, err = s.Verify(strings.Repeat("a", 64), peer); err == nil {
		t.Fatal("forged token accepted")
	}
	data, _ := json.Marshal(sink.events)
	for _, secret := range []string{login.Token, "GoodPassword42", "password_hash", "console_session"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("credential leaked into audit")
		}
	}
	if err = s.Logout(context.Background(), login.Token, peer); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(login.Token, peer); err == nil {
		t.Fatal("logged out session accepted")
	}
	login, err = s.Login(context.Background(), peer, "alice", []byte("GoodPassword42"))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err = s.Verify(login.Token, peer); err == nil {
		t.Fatal("expired session accepted")
	}
}
func TestCredentialChangesAndLegacyMappingsCannotBypassAuthentication(t *testing.T) {
	s, _ := fixture(t)
	p := Peer{1000, 123}
	ctx := context.Background()
	if _, err := s.Login(ctx, p, "legacy", []byte("GoodPassword42")); err == nil {
		t.Fatal("legacy OS mapping authenticated")
	}
	login, err := s.Login(ctx, p, "alice", []byte("GoodPassword42"))
	if err != nil {
		t.Fatal(err)
	}
	users, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	users.Users[0].Roles = []string{"netops"}
	data, _ := json.Marshal(users)
	if err = os.WriteFile(s.usersPath, data, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(login.Token, p); err == nil {
		t.Fatal("role change did not revoke session")
	}
}
func TestLoginIsRateLimitedAndAuditFailureFailsClosed(t *testing.T) {
	s, sink := fixture(t)
	p := Peer{1000, 123}
	ctx := context.Background()
	sink.fail = true
	if _, err := s.Login(ctx, p, "alice", []byte("GoodPassword42")); err == nil {
		t.Fatal("audit failure accepted login")
	}
	sink.fail = false
	for i := 0; i < 5; i++ {
		if _, err := s.Login(ctx, p, "alice", []byte("wrong")); err == nil {
			t.Fatal("bad password accepted")
		}
	}
	if _, err := s.Login(ctx, p, "alice", []byte("GoodPassword42")); err == nil {
		t.Fatal("rate limit bypassed with correct password")
	}
}
