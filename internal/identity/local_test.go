package identity

import (
	"context"
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func localFixture(t *testing.T) (*Service, *recorder, LocalUsers, Peer) {
	t.Helper()
	account, err := user.Current()
	if err != nil || os.Getuid() == 0 || reservedLocalUser(account.Username) {
		t.Skip("requires individual non-root test user")
	}
	s, sink := fixture(t)
	u := LocalUsers{Users: []LocalUser{{OSUser: account.Username, UserID: "local-alice", Roles: []string{"viewer"}}}}
	writeLocalFixture(t, s, u)
	return s, sink, u, Peer{UID: uint32(os.Getuid()), PID: int32(os.Getpid())}
}
func writeLocalFixture(t *testing.T, s *Service, u LocalUsers) {
	t.Helper()
	data, _ := json.Marshal(u)
	if err := os.WriteFile(filepath.Join(filepath.Dir(s.usersPath), "linux-users.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}
func TestLinuxLoginUsesKernelIdentityAndRevokesChangedMappings(t *testing.T) {
	s, sink, users, p := localFixture(t)
	ctx := context.Background()
	session, err := s.LoginLocal(ctx, p)
	if err != nil || session.UserID != "local-alice" {
		t.Fatal(session, err)
	}
	if _, err := s.Verify(session.Token, p); err != nil {
		t.Fatal(err)
	}
	for _, other := range []Peer{{p.UID + 1, p.PID}, {p.UID, p.PID + 1}} {
		if _, err := s.Verify(session.Token, other); err == nil {
			t.Fatal("token replay accepted")
		}
	}
	if _, err := s.LoginLocal(ctx, Peer{p.UID + 1, p.PID}); err == nil {
		t.Fatal("unmapped UID logged in")
	}
	users.Users[0].Roles = []string{"netops"}
	writeLocalFixture(t, s, users)
	if _, err := s.Verify(session.Token, p); err == nil {
		t.Fatal("role change failed to revoke session")
	}
	session, err = s.LoginLocal(ctx, p)
	if err != nil || strings.Join(session.Roles, ",") != "netops" {
		t.Fatal(err)
	}
	users.Users[0].Disabled = true
	writeLocalFixture(t, s, users)
	if _, err := s.Verify(session.Token, p); err == nil {
		t.Fatal("disabled mapping retained session")
	}
	if _, err := s.LoginLocal(ctx, p); err == nil {
		t.Fatal("disabled mapping logged in")
	}
	users.Users[0].Disabled = false
	writeLocalFixture(t, s, users)
	sink.mu.Lock()
	sink.fail = true
	sink.mu.Unlock()
	if _, err := s.LoginLocal(ctx, p); err == nil {
		t.Fatal("login ignored failed audit")
	}
}

func TestLinuxLoginIPCRejectsClaimedIdentityAndVerifierAccess(t *testing.T) {
	s, _, _, p := localFixture(t)
	path := filepath.Join(t.TempDir(), "local.sock")
	server, err := Listen(path, s, p.UID+1, []uint32{p.UID + 2})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if err := server.SetLocalUIDs([]uint32{p.UID}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go server.Serve(ctx)
	client := NewClient(path, p.UID)
	session, err := client.LoginLocal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Login(ctx, "alice", []byte("GoodPassword42")); err == nil {
		t.Fatal("Linux UID used shared internal login")
	}
	if _, err := client.call(ctx, request{Action: "local-login", UserID: "alice"}); err == nil {
		t.Fatal("claimed identity accepted")
	}
	if _, err := client.call(ctx, request{Action: "local-login", PeerUID: p.UID + 1, PeerPID: p.PID}); err == nil {
		t.Fatal("claimed peer accepted")
	}
	if _, err := client.Verify(ctx, session.Token, p); err == nil {
		t.Fatal("operator used privileged verifier")
	}
	if err := client.Logout(ctx, session.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(session.Token, p); err == nil {
		t.Fatal("logout failed")
	}
}

func TestLinuxMappingsRejectUnsafeFilesAndReservedAccounts(t *testing.T) {
	s, _, users, _ := localFixture(t)
	for _, name := range []string{"root", "safeterm", "safeterm-gateway"} {
		users.Users[0].OSUser = name
		data, _ := json.Marshal(users)
		if _, err := ParseLocalUsers(data, s.roles); err == nil {
			t.Fatal("reserved account", name)
		}
	}
	path := filepath.Join(filepath.Dir(s.usersPath), "linux-users.json")
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLocalUsers(path, s.roles); err == nil {
		t.Fatal("writable authorization file accepted")
	}
}
