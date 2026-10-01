//go:build linux

package integration

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"safeterm/internal/agent"
	"safeterm/internal/audit"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/execution"
	"safeterm/internal/integrity"
	"safeterm/internal/isolation"
	"safeterm/internal/policy"
	"safeterm/internal/translation"
	"safeterm/internal/validation"
)

type streamingAuditSink struct {
	mu     sync.Mutex
	events []audit.Event
}

func (s *streamingAuditSink) Record(_ context.Context, event audit.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	return nil
}

func (s *streamingAuditSink) has(outcome string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range s.events {
		if event.Outcome == outcome {
			return true
		}
	}
	return false
}

// TEST-STREAM-016
func TestVerifiedCoreStreamsAcrossGatewayAndTerminatesSemantically(t *testing.T) {
	t.Setenv(executionIntegrationEnvironment, "1")
	executablePath, checksum := copyExecutable(t)
	cfg := executionIntegrationConfig()
	cfg.Executables[0].Path = executablePath
	cfg.Executables[0].SHA256 = checksum
	for index := range cfg.Operations {
		if cfg.Operations[index].Name == "probe_wait" {
			cfg.Operations[index].LongRunning = true
		}
	}
	if err := config.ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	store, err := isolation.NewStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	translator, err := translation.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	executionOptions := execution.DefaultOptions(10 * time.Second)
	executionOptions.ChunkBytes = 8
	executionOptions.OutputLimit = 4096
	executionOptions.InterruptGrace = 50 * time.Millisecond
	executionOptions.TerminateGrace = 50 * time.Millisecond
	runner, err := execution.NewWithOptions(executionOptions)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := integrity.NewExecutor(cfg, config.ProfileDiagnostics, runner)
	if err != nil {
		t.Fatal(err)
	}
	core := agent.NewCore(validation.New(cfg), policy.New(cfg), translator, verified)
	auditSink := &streamingAuditSink{}
	handler, err := isolation.NewProfileHandlerWithSink(config.ProfileDiagnostics, store, core, auditSink)
	if err != nil {
		t.Fatal(err)
	}
	streamOptions := isolation.DefaultStreamOptions()
	streamOptions.ChunkBytes = 8
	streamOptions.OutputBudget = 4096
	streamOptions.IdleTimeout = time.Second
	streamOptions.HistoryTTL = time.Minute
	manager, err := isolation.NewSessionManager(config.ProfileDiagnostics, store, handler, auditSink, streamOptions)
	if err != nil {
		t.Fatal(err)
	}
	uid := uint32(os.Getuid())
	path := filepath.Join(t.TempDir(), "runner.sock")
	server, err := isolation.ListenUnixStreaming(path, 0o600, []uint32{uid}, handler, manager)
	if err != nil {
		t.Fatal(err)
	}
	serveCtx, stopServer := context.WithCancel(context.Background())
	t.Cleanup(func() {
		stopServer()
		server.Close()
	})
	go server.Serve(serveCtx)
	client, err := isolation.NewUnixClient(path, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := isolation.NewGatewayWithSink(store, map[config.RunnerProfile]isolation.Client{config.ProfileDiagnostics: client}, auditSink)
	if err != nil {
		t.Fatal(err)
	}
	request := executionRequest(t, "probe_wait", []string{"operator"}, nil)
	request.Metadata.BrokerType = "web"
	request.Metadata.UserID = "alice"
	session, err := gateway.StartStream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr []byte
	deadline := time.After(2 * time.Second)
	for len(stdout) < len("integration-stdout-before-wait") || len(stderr) < len("integration-stderr-before-wait") {
		select {
		case frame := <-session.Events:
			switch frame.Type {
			case contract.FrameStdout:
				stdout = append(stdout, frame.Data...)
			case contract.FrameStderr:
				stderr = append(stderr, frame.Data...)
			}
		case <-deadline:
			t.Fatalf("missing streamed output stdout=%q stderr=%q", stdout, stderr)
		}
	}
	control := contract.ControlRequest{Version: contract.StreamProtocolVersion, Action: contract.ControlTerminate, SessionID: session.ID, Owner: contract.OwnerFromMetadata(request.Metadata)}
	result, err := gateway.Terminate(context.Background(), control)
	if err != nil || result.Status != contract.ControlTerminating {
		t.Fatalf("terminate result=%+v error=%v", result, err)
	}
	var final *contract.ExecutionResponse
	for frame := range session.Events {
		if frame.Type == contract.FrameSessionExited {
			final = frame.Response
		}
	}
	if final == nil || final.Result != contract.ResultExecutionError || final.Stdout != "" || final.Stderr != "" {
		t.Fatalf("final response=%+v", final)
	}
	for _, outcome := range []string{"routed", "accepted", "session-started", "stream-opened", "terminate-requested", "interrupt-delivered", "completed", "session-exited"} {
		if !auditSink.has(outcome) {
			t.Fatalf("missing audit outcome %q", outcome)
		}
	}
}
