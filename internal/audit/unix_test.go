//go:build linux

package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type memorySink struct{ events []Event }

func (s *memorySink) Record(_ context.Context, event Event) error {
	s.events = append(s.events, event)
	return nil
}

// TEST-AUDIT-005
func TestUnixIPCAuthenticatesPeersAndAcknowledgesPersistence(t *testing.T) {
	uid := uint32(os.Getuid())
	sink := &memorySink{}
	path := filepath.Join(t.TempDir(), "audit.sock")
	server, err := ListenUnix(path, 0o600, map[uint32][]Producer{uid: {{Component: ComponentRunner, Profile: "network-observer"}}}, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	client, err := NewUnixClient(path, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Record(context.Background(), testEvent()); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 || sink.events[0].RequestID == "" {
		t.Fatalf("events=%+v", sink.events)
	}
}

// TEST-AUDIT-005, TEST-AUDIT-008
func TestUnixIPCRejectsUntrustedPeerAndBadConstruction(t *testing.T) {
	uid := uint32(os.Getuid())
	path := filepath.Join(t.TempDir(), "audit.sock")
	server, err := ListenUnix(path, 0o600, map[uint32][]Producer{uid + 1: {{Component: ComponentRunner, Profile: "network-observer"}}}, &memorySink{})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	client, _ := NewUnixClient(path, []uint32{uid})
	if err := client.Record(context.Background(), testEvent()); err == nil {
		t.Fatal("untrusted peer accepted")
	}
	if _, err := NewUnixClient("", nil); err == nil {
		t.Fatal("invalid client accepted")
	}
	if _, err := ListenUnix("", 0, nil, nil); err == nil {
		t.Fatal("invalid server accepted")
	}
}

// TEST-AUDIT-005
func TestUnixIPCRejectsProducerSourceSpoofing(t *testing.T) {
	uid := uint32(os.Getuid())
	path := filepath.Join(t.TempDir(), "audit.sock")
	server, err := ListenUnix(path, 0o600, map[uint32][]Producer{uid: {{Component: ComponentRunner, Profile: "diagnostics"}}}, &memorySink{})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	client, _ := NewUnixClient(path, []uint32{uid})
	event := testEvent()
	if err := client.Record(context.Background(), event); err == nil {
		t.Fatal("cross-profile producer event accepted")
	}
	event.Component = ComponentGateway
	if err := client.Record(context.Background(), event); err == nil {
		t.Fatal("forged Gateway event accepted")
	}
}
