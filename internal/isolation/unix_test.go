//go:build linux

package isolation

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"safeterm/internal/contract"
)

// TEST-ISOLATION-006, TEST-ISOLATION-010
func TestUnixIPCAuthenticatesPeersAndPreservesRequestID(t *testing.T) {
	uid := uint32(os.Getuid())
	path := filepath.Join(t.TempDir(), "runner.sock")
	handler := &fakeClient{response: &contract.ExecutionResponse{RequestID: "request-1", Result: contract.ResultSuccess}}
	server, err := ListenUnix(path, 0o600, []uint32{uid}, handler)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	client, err := NewUnixClient(path, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}
	request := &contract.ExecutionRequest{RequestID: "request-1", Command: "system_info"}
	response, err := client.Handle(context.Background(), request)
	if err != nil || response.RequestID != request.RequestID || handler.calls != 1 {
		t.Fatalf("response=%+v error=%v calls=%d", response, err, handler.calls)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode=%v error=%v", info.Mode().Perm(), err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
}

func TestUnixIPCRejectsUntrustedPeer(t *testing.T) {
	uid := uint32(os.Getuid())
	path := filepath.Join(t.TempDir(), "runner.sock")
	handler := &fakeClient{response: &contract.ExecutionResponse{Result: contract.ResultSuccess}}
	server, err := ListenUnix(path, 0o600, []uint32{uid + 1}, handler)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	client, _ := NewUnixClient(path, []uint32{uid})
	if _, err := client.Handle(context.Background(), &contract.ExecutionRequest{Command: "system_info"}); err == nil {
		t.Fatal("untrusted client was accepted")
	}
	if handler.calls != 0 {
		t.Fatal("handler reached by untrusted client")
	}
}

func TestUnixIPCRejectsUnknownFieldsAndTrailingMessages(t *testing.T) {
	uid := uint32(os.Getuid())
	for _, payload := range []string{
		"{\"request_id\":\"one\",\"command\":\"system_info\",\"unexpected\":true}\n",
		"{\"request_id\":\"one\",\"command\":\"system_info\"} {\"request_id\":\"two\",\"command\":\"system_info\"}\n",
	} {
		path := filepath.Join(t.TempDir(), "runner.sock")
		handler := &fakeClient{response: &contract.ExecutionResponse{Result: contract.ResultSuccess}}
		server, err := ListenUnix(path, 0o600, []uint32{uid}, handler)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		go server.Serve(ctx)
		conn, err := net.Dial("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Write([]byte(payload)); err != nil {
			t.Fatal(err)
		}
		var reply ipcReply
		if err := json.NewDecoder(conn).Decode(&reply); err != nil {
			t.Fatal(err)
		}
		conn.Close()
		cancel()
		server.Close()
		if reply.Error != "invalid request" || handler.calls != 0 {
			t.Fatalf("payload accepted: reply=%+v calls=%d", reply, handler.calls)
		}
	}
}
