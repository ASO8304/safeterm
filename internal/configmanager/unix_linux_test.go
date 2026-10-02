//go:build linux

package configmanager

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
