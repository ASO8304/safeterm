//go:build linux

package configmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIPCAuthenticatesActorBeforeDispatchAndReplacesRoles(t *testing.T) {
	manager, err := NewManager("application", testRegistry(writeTestINI(t), false), nil, nil, &recordingSink{}, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "authenticated.sock")
	uid, pid := uint32(os.Getuid()), int32(os.Getpid())
	server, err := ListenUnix(socket, 0600, []uint32{uid}, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	server.SetActorAuthorizer(func(actor *Actor, peerUID uint32, peerPID int32) error {
		if peerUID != uid || peerPID != pid || actor.UserID != "alice" || actor.ConsoleSession != "valid-session" {
			return fmt.Errorf("denied")
		}
		actor.Roles = []string{"viewer"}
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	client, _ := NewUnixClient(socket, []uint32{uid})
	claim := Actor{BrokerType: "console", UserID: "alice", Roles: []string{"operator", "viewer"}}
	if _, err = client.Read(ctx, claim, "cats-options"); err == nil {
		t.Fatal("shared UID read resource without login")
	}
	claim.ConsoleSession = "valid-session"
	read, err := client.Read(ctx, claim, "cats-options")
	if err != nil {
		t.Fatal(err)
	}
	patch := Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: read.Version}
	if _, err = client.Preview(ctx, claim, patch); err == nil {
		t.Fatal("caller-supplied operator role was trusted")
	}
}

func TestUnixIPCAuthenticatesPeersAndCarriesSemanticRequests(t *testing.T) {
	path := writeTestINI(t)
	manager, err := NewManager("application", testRegistry(path, false), nil, nil, &recordingSink{}, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "manager.sock")
	server, err := ListenUnix(socket, 0o600, []uint32{uint32(os.Getuid())}, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx) }()
	client, _ := NewUnixClient(socket, []uint32{uint32(os.Getuid())})
	callCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	resources, err := client.List(callCtx, actor("alice", "viewer"))
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].ID != "cats-options" {
		t.Fatalf("resources=%+v", resources)
	}
	read, err := client.Read(callCtx, actor("alice", "viewer"), "cats-options")
	if err != nil || read.Version == "" {
		t.Fatalf("read=%+v err=%v", read, err)
	}
}

// TEST-SECURITY-014: REQ-014 AC-5.
func TestConfigManagerIPCRejectsAmbiguousUnknownAndOversizedRequests(t *testing.T) {
	path := writeTestINI(t)
	manager, err := NewManager("application", testRegistry(path, false), nil, nil, &recordingSink{}, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "strict.sock")
	server, err := ListenUnix(socket, 0o600, []uint32{uint32(os.Getuid())}, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	actorJSON := `"actor":{"broker_type":"ssh","user_id":"alice","roles":["viewer"]}`
	payloads := []string{`{"version":1,"action":"list",` + actorJSON + `,"resource_id":"cats-options"}` + "\n", `{"version":1,"action":"read",` + actorJSON + `,"resource_id":"cats-options","unexpected":true}` + "\n", `{"version":1,"action":"apply",` + actorJSON + `,"token":"bad"}` + "\n", `{"version":1,"action":"list",` + actorJSON + `}` + strings.Repeat(" ", maxIPCMessageBytes) + "\n"}
	for _, payload := range payloads {
		connection, err := net.DialTimeout("unix", socket, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = connection.SetDeadline(time.Now().Add(time.Second))
		if _, err := connection.Write([]byte(payload)); err != nil {
			connection.Close()
			t.Fatal(err)
		}
		var reply ipcReply
		if err := json.NewDecoder(connection).Decode(&reply); err != nil {
			connection.Close()
			t.Fatal(err)
		}
		connection.Close()
		if reply.Error == "" {
			t.Fatalf("invalid payload accepted: %.120q", payload)
		}
	}
}

// TEST-SECURITY-017: REQ-014 AC-5 and AC-8.
func TestConfigManagerIPCConnectionAdmissionIsBounded(t *testing.T) {
	server := &UnixServer{connections: make(chan struct{}, maxConcurrentIPCConnections)}
	for i := 0; i < maxConcurrentIPCConnections; i++ {
		if !server.acquireConnection() {
			t.Fatalf("connection %d rejected before limit", i)
		}
	}
	if server.acquireConnection() {
		t.Fatal("connection above limit was accepted")
	}
	server.releaseConnection()
	if !server.acquireConnection() {
		t.Fatal("released connection slot was not reusable")
	}
}
