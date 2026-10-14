//go:build linux

package main

import (
	"context"
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"os"
	"path/filepath"
	"safeterm/internal/audit"
	"safeterm/internal/broker"
	"safeterm/internal/contract"
	"safeterm/internal/identity"
	"safeterm/internal/isolation"
	"testing"
	"time"
)

type consoleTestSink struct{}

func (consoleTestSink) Record(context.Context, audit.Event) error { return nil }
func TestConsoleGatewayDerivesRolesAndRejectsForgedIdentity(t *testing.T) {
	uid := uint32(os.Getuid())
	peer := identity.Peer{UID: uid + 1, PID: 1234}
	hash, _ := bcrypt.GenerateFromPassword([]byte("GoodPassword42"), 12)
	path := filepath.Join(t.TempDir(), "users.json")
	data, _ := json.Marshal(broker.ConsoleIdentityConfig{Users: []broker.ConsoleIdentity{{UserID: "alice", Roles: []string{"viewer"}, PasswordHash: string(hash)}}})
	if err := os.WriteFile(path, data, 0640); err != nil {
		t.Fatal(err)
	}
	service, err := identity.New(path, map[string]bool{"viewer": true}, time.Minute, consoleTestSink{}, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.Login(context.Background(), peer, "alice", []byte("GoodPassword42"))
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "identity.sock")
	server, err := identity.Listen(socket, service, uid+1, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	policy := consolePeerPolicy(identity.NewClient(socket, uid))
	request, _ := contract.NewExecutionRequest("system_info", nil, contract.Metadata{BrokerType: "console", UserID: "alice", ClaimedRoles: []string{"service-operator"}, ConsoleSession: session.Token})
	if err = policy.AuthorizePeer(request, isolation.PeerIdentity{UID: peer.UID, PID: peer.PID}); err != nil {
		t.Fatal(err)
	}
	if len(request.Metadata.ClaimedRoles) != 1 || request.Metadata.ClaimedRoles[0] != "viewer" || request.Metadata.SessionExpiresAt.IsZero() {
		t.Fatal("caller controlled roles or expiry")
	}
	request.Metadata.UserID = "admin"
	if policy.AuthorizePeer(request, isolation.PeerIdentity{UID: peer.UID, PID: peer.PID}) == nil {
		t.Fatal("forged user accepted")
	}
	request.Metadata.UserID = "alice"
	if policy.AuthorizePeer(request, isolation.PeerIdentity{UID: peer.UID, PID: peer.PID + 1}) == nil {
		t.Fatal("another process replayed session")
	}
	request.Metadata.ConsoleSession = ""
	if policy.AuthorizePeer(request, isolation.PeerIdentity{UID: peer.UID, PID: peer.PID}) == nil {
		t.Fatal("UID-only access accepted")
	}
	control := &contract.ControlRequest{Owner: contract.SessionOwner{BrokerType: "console", UserID: "admin", ConsoleSession: session.Token}}
	if policy.AuthorizeControl(control, isolation.PeerIdentity{UID: peer.UID, PID: peer.PID}) == nil {
		t.Fatal("forged control owner accepted")
	}
}
