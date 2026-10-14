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
	"safeterm/internal/configmanager"
	"safeterm/internal/identity"
	"testing"
	"time"
)

type consoleTestSink struct{}

func (consoleTestSink) Record(context.Context, audit.Event) error { return nil }
func TestConfigManagerDerivesRolesAndRejectsForgedIdentity(t *testing.T) {
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
	authorize := consoleActorAuthorizer(peer.UID, identity.NewClient(socket, uid))
	actor := &configmanager.Actor{UserID: "alice", BrokerType: "console", Roles: []string{"service-operator"}, ConsoleSession: session.Token}
	if err = authorize(actor, peer.UID, peer.PID); err != nil {
		t.Fatal(err)
	}
	if len(actor.Roles) != 1 || actor.Roles[0] != "viewer" {
		t.Fatal("caller controlled roles")
	}
	actor.UserID = "admin"
	if authorize(actor, peer.UID, peer.PID) == nil {
		t.Fatal("forged user accepted")
	}
	actor.UserID = "alice"
	if authorize(actor, peer.UID, peer.PID+1) == nil {
		t.Fatal("another process replayed session")
	}
	actor.ConsoleSession = ""
	if authorize(actor, peer.UID, peer.PID) == nil {
		t.Fatal("UID-only access accepted")
	}
	actor.BrokerType = "ssh"
	if authorize(actor, peer.UID, peer.PID) == nil {
		t.Fatal("console UID impersonated SSH Broker")
	}
	actor.BrokerType = "console"
	if authorize(actor, uid, peer.PID) == nil {
		t.Fatal("SSH peer impersonated Console")
	}
}
