//go:build linux

package configmanager

import (
	"context"
	"os"
	"path/filepath"
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
