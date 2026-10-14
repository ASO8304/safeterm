//go:build linux

package isolation

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"safeterm/internal/contract"
)

func TestConsoleStreamRechecksWithoutInjectingFramesAndRevokes(t *testing.T) {
	uid := uint32(os.Getuid())
	manager, recorder := newTestSessionManagerWithTimeout(t, "interrupt", DefaultStreamOptions(), 45*time.Second)
	var revoked atomic.Bool
	var calls atomic.Int32
	rechecked := make(chan struct{}, 1)
	policy := PeerPolicy{AllowStreaming: true, AuthorizePeer: func(*contract.ExecutionRequest, PeerIdentity) error {
		if revoked.Load() {
			return fmt.Errorf("session revoked")
		}
		if calls.Add(1) > 1 {
			select {
			case rechecked <- struct{}{}:
			default:
			}
		}
		return nil
	}}
	path := filepath.Join(t.TempDir(), "revocation.sock")
	server, err := ListenUnixStreamingWithPeerPolicies(path, 0600, map[uint32]PeerPolicy{uid: policy}, manager.handler, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	go server.Serve(ctx)
	client, _ := NewUnixClient(path, []uint32{uid})
	session, err := client.StartStream(ctx, streamRequest(t, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-rechecked:
	case <-ctx.Done():
		t.Fatal("stream was not revalidated")
	}
	// Successful verification is not a stream event. An empty injected frame
	// would make the real Unix client close this stream immediately.
	stable := time.After(300 * time.Millisecond)
drain:
	for {
		select {
		case _, ok := <-session.Events:
			if !ok {
				t.Fatal("successful recheck broke streaming")
			}
		case <-stable:
			break drain
		}
	}
	revoked.Store(true)
	for {
		select {
		case _, ok := <-session.Events:
			if !ok {
				if !recorder.has("client-disconnected") {
					t.Fatal("revocation did not cancel the session")
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("revoked stream remained active")
		}
	}
}

func TestUnixWaitsCancelWithoutWaitingForDeadline(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "stalled.sock")
			listener, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			received := make(chan struct{})
			serverDone := make(chan struct{})
			go func() {
				defer close(serverDone)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				var request json.RawMessage
				if json.NewDecoder(conn).Decode(&request) != nil {
					return
				}
				close(received)
				var b [1]byte
				_, _ = conn.Read(b[:]) // Never respond; wait for client cancellation.
			}()
			client, err := NewUnixClient(path, []uint32{uint32(os.Getuid())})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request, _ := contract.NewExecutionRequest("system_info", nil, contract.Metadata{})
			finished := make(chan error, 1)
			go func() {
				if streaming {
					_, err := client.StartStream(ctx, request)
					finished <- err
				} else {
					_, err := client.Handle(ctx, request)
					finished <- err
				}
			}()
			select {
			case <-received:
			case <-time.After(2 * time.Second):
				t.Fatal("request did not arrive")
			}
			cancel()
			select {
			case err := <-finished:
				if err == nil {
					t.Fatal("stalled request unexpectedly succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation did not interrupt IPC wait")
			}
			<-serverDone
		})
	}
}

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

// TEST-SECURITY-015: REQ-014 AC-5 and AC-8.
func TestUnixIPCConnectionAdmissionIsBounded(t *testing.T) {
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

// TEST-SECURITY-011: REQ-014 AC-4 and AC-5.
func TestUnixPeerPolicyRestrictsUnaryCommandsAndStreaming(t *testing.T) {
	uid := uint32(os.Getuid())
	path := filepath.Join(t.TempDir(), "policy.sock")
	options := DefaultStreamOptions()
	options.IdleTimeout = time.Second
	manager, _ := newTestSessionManager(t, "interrupt", options)
	policy := PeerPolicy{Authorize: func(request *contract.ExecutionRequest) error {
		if request.Command != "allowed" || len(request.Arguments) != 0 {
			return fmt.Errorf("denied")
		}
		return nil
	}}
	server, err := ListenUnixStreamingWithPeerPolicies(path, 0o600, map[uint32]PeerPolicy{uid: policy}, manager.handler, manager)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	client, _ := NewUnixClient(path, []uint32{uid})
	if _, err := client.Handle(context.Background(), &contract.ExecutionRequest{RequestID: "denied", Command: "other"}); err == nil {
		t.Fatal("peer policy allowed an unregistered command")
	}
	if _, err := client.Handle(context.Background(), &contract.ExecutionRequest{RequestID: "allowed", Command: "allowed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StartStream(context.Background(), &contract.ExecutionRequest{RequestID: "stream", Command: "allowed"}); err == nil {
		t.Fatal("non-streaming peer opened a stream")
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

func TestStreamingCannotBypassPeerAuthentication(t *testing.T) {
	uid := uint32(os.Getuid())
	manager, _ := newTestSessionManager(t, "interrupt", DefaultStreamOptions())
	policy := PeerPolicy{AllowStreaming: true, AuthorizePeer: func(_ *contract.ExecutionRequest, peer PeerIdentity) error {
		if peer.UID != uid || peer.PID != int32(os.Getpid()) {
			t.Error("peer identity was not derived from the kernel")
		}
		return fmt.Errorf("no authenticated session")
	}, AuthorizeControl: func(*contract.ControlRequest, PeerIdentity) error { return fmt.Errorf("no authenticated session") }}
	path := filepath.Join(t.TempDir(), "gateway.sock")
	server, err := ListenUnixStreamingWithPeerPolicies(path, 0600, map[uint32]PeerPolicy{uid: policy}, manager.handler, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	client, _ := NewUnixClient(path, []uint32{uid})
	if _, err = client.StartStream(ctx, streamRequest(t, "alice")); err == nil {
		t.Fatal("stream bypassed authentication")
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	if manager.active != 0 || len(manager.sessions) != 0 {
		t.Fatal("unauthenticated request created a session")
	}
}

// TEST-STREAM-014
func TestUnixIPCStreamsAndControlsSessionOverSeparateConnection(t *testing.T) {
	uid := uint32(os.Getuid())
	path := filepath.Join(t.TempDir(), "stream.sock")
	options := DefaultStreamOptions()
	options.ChunkBytes = 8
	options.OutputBudget = 1024
	options.IdleTimeout = time.Second
	options.HistoryTTL = time.Minute
	manager, _ := newTestSessionManager(t, "interrupt", options)
	server, err := ListenUnixStreaming(path, 0o600, []uint32{uid}, manager.handler, manager)
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
	request := streamRequest(t, "alice")
	session, err := client.StartStream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	seenOutput := false
	for !seenOutput {
		select {
		case frame := <-session.Events:
			seenOutput = frame.Type == contract.FrameStdout && len(frame.Data) > 0
		case <-time.After(2 * time.Second):
			t.Fatal("stream output did not cross Unix IPC")
		}
	}
	control := contract.ControlRequest{Version: contract.StreamProtocolVersion, Action: contract.ControlTerminate, SessionID: session.ID, Owner: contract.OwnerFromMetadata(request.Metadata)}
	result, err := client.Terminate(context.Background(), control)
	if err != nil || result.Status != contract.ControlTerminating {
		t.Fatalf("terminate result=%+v error=%v", result, err)
	}
	seenExit := false
	for frame := range session.Events {
		if frame.Type == contract.FrameSessionExited {
			seenExit = true
		}
	}
	if !seenExit {
		t.Fatal("session exit did not cross Unix IPC")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("streaming server did not stop")
	}
}

// TEST-STREAM-020
func TestUnixStreamingClientDisconnectTerminatesSession(t *testing.T) {
	uid := uint32(os.Getuid())
	path := filepath.Join(t.TempDir(), "disconnect.sock")
	options := DefaultStreamOptions()
	options.IdleTimeout = time.Second
	manager, recorder := newTestSessionManager(t, "ignore", options)
	server, err := ListenUnixStreaming(path, 0o600, []uint32{uid}, manager.handler, manager)
	if err != nil {
		t.Fatal(err)
	}
	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go server.Serve(serverCtx)
	client, _ := NewUnixClient(path, []uint32{uid})
	clientCtx, disconnect := context.WithCancel(context.Background())
	session, err := client.StartStream(clientCtx, streamRequest(t, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	for {
		frame := <-session.Events
		if frame.Type == contract.FrameStdout && len(frame.Data) > 0 {
			break
		}
	}
	disconnect()
	deadline := time.Now().Add(2 * time.Second)
	for (!recorder.has("client-disconnected") || !recorder.has("session-exited")) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !recorder.has("client-disconnected") || !recorder.has("session-exited") {
		t.Fatalf("disconnect lifecycle missing: %+v", recorder.events)
	}
}

// TEST-STREAM-022
func TestUnixStreamingRejectsUnknownFieldsAndRawSignalControls(t *testing.T) {
	uid := uint32(os.Getuid())
	options := DefaultStreamOptions()
	manager, _ := newTestSessionManager(t, "interrupt", options)
	for _, payload := range []string{
		"{\"version\":1,\"type\":\"start\",\"request\":{\"request_id\":\"one\",\"command\":\"trace\"},\"unexpected\":true}\n",
		"{\"version\":1,\"type\":\"terminate\",\"control\":{\"version\":1,\"action\":\"terminate\",\"session_id\":\"opaque\",\"owner\":{},\"signal\":9}}\n",
	} {
		path := filepath.Join(t.TempDir(), "invalid-stream.sock")
		server, err := ListenUnixStreaming(path, 0o600, []uint32{uid}, manager.handler, manager)
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
		var reply streamIPCReply
		if err := json.NewDecoder(conn).Decode(&reply); err != nil {
			t.Fatal(err)
		}
		conn.Close()
		cancel()
		server.Close()
		if reply.Error != "invalid stream request" {
			t.Fatalf("invalid payload reply = %+v", reply)
		}
	}
}
