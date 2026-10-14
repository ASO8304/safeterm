//go:build linux

package identity

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnixLoginRejectsVerifierCallsFromSharedAccountAndSpoofedServer(t *testing.T) {
	s, _ := fixture(t)
	uid := uint32(os.Getuid())
	path := filepath.Join(t.TempDir(), "identity.sock")
	server, err := Listen(path, s, uid, []uint32{uid + 1})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	client := NewClient(path, uid)
	session, err := client.Login(ctx, "alice", []byte("GoodPassword42"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Verify(ctx, session.Token, Peer{uid, int32(os.Getpid())}); err == nil {
		t.Fatal("shared UID used privileged verification API")
	}
	if _, err = NewClient(path, uid+1).Login(ctx, "alice", []byte("GoodPassword42")); err == nil {
		t.Fatal("spoofed identity server accepted")
	}
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("{\"version\":1,\"action\":\"login\",\"roles\":[\"netops\"]}\n"))
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	n, _ := conn.Read(b[:])
	conn.Close()
	if n != 0 {
		t.Fatal("unknown roles field accepted")
	}
	if err = client.Logout(ctx, session.Token); err != nil {
		t.Fatal(err)
	}
}
func TestTrustedVerifierCannotChangeTokenPeerBinding(t *testing.T) {
	s, _ := fixture(t)
	uid := uint32(os.Getuid())
	p := Peer{uid + 1, 1234}
	session, err := s.Login(context.Background(), p, "alice", []byte("GoodPassword42"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "verify.sock")
	server, err := Listen(path, s, uid+1, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	client := NewClient(path, uid)
	got, err := client.Verify(ctx, session.Token, p)
	if err != nil || got.UserID != "alice" || got.Token != "" {
		t.Fatalf("verify=%+v err=%v", got, err)
	}
	if _, err = client.Verify(ctx, session.Token, Peer{uid + 1, 1235}); err == nil {
		t.Fatal("PID binding bypassed")
	}
	if _, err = client.Login(ctx, "alice", []byte("GoodPassword42")); err == nil {
		t.Fatal("verifier issued login as console")
	}
}
