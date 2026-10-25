//go:build linux

package isolation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"safeterm/internal/agent"
	"safeterm/internal/audit"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/execution"
)

const sessionHelperEnvironment = "SAFETERM_SESSION_HELPER"

func TestSessionHelperProcess(t *testing.T) {
	if os.Getenv(sessionHelperEnvironment) != "1" {
		return
	}
	separator := -1
	for index, value := range os.Args {
		if value == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(91)
	}
	switch os.Args[separator+1] {
	case "interrupt":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt)
		fmt.Fprint(os.Stdout, "ready")
		fmt.Fprint(os.Stderr, "diagnostic")
		<-signals
		fmt.Fprint(os.Stdout, "stopped")
		os.Exit(0)
	case "ignore":
		signal.Ignore(os.Interrupt, syscall.SIGTERM)
		fmt.Fprint(os.Stdout, "ready")
		for {
			time.Sleep(time.Hour)
		}
	case "flood":
		for index := 0; index < 4096; index++ {
			fmt.Fprint(os.Stdout, "0123456789abcdef")
		}
		os.Exit(0)
	case "success":
		fmt.Fprint(os.Stdout, "done")
		os.Exit(0)
	default:
		os.Exit(92)
	}
}

type allowStage struct{}

func TestConsoleSessionExpiryTerminatesRunningProcess(t *testing.T) {
	manager, _ := newTestSessionManager(t, "interrupt", DefaultStreamOptions())
	request := streamRequest(t, "alice")
	request.Metadata.BrokerType = "console"
	request.Metadata.ConsoleSession = "test-session"
	request.Metadata.SessionExpiresAt = time.Now().Add(500 * time.Millisecond)
	session, err := manager.StartStream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	timeout := time.After(5 * time.Second)
	for {
		select {
		case frame, ok := <-session.Events:
			if !ok {
				t.Fatal("missing session exit")
			}
			if frame.Type == contract.FrameSessionExited {
				return
			}
		case <-timeout:
			t.Fatal("operation survived Console authentication expiry")
		}
	}
}

func (allowStage) Validate(*contract.ExecutionRequest) error  { return nil }
func (allowStage) Authorize(*contract.ExecutionRequest) error { return nil }

type helperTranslator struct{ action string }

func (t helperTranslator) Translate(*contract.ExecutionRequest) (string, []string, error) {
	return os.Args[0], []string{"-test.run=^TestSessionHelperProcess$", "--", t.action}, nil
}

type lockedEventRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

type failSessionExitSink struct{}

func (failSessionExitSink) Record(_ context.Context, event audit.Event) error {
	if event.Outcome == "session-exited" {
		return fmt.Errorf("audit unavailable")
	}
	return event.Validate()
}

func (r *lockedEventRecorder) Record(_ context.Context, event audit.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
	return nil
}

func (r *lockedEventRecorder) has(outcome string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, event := range r.events {
		if event.Outcome == outcome {
			return true
		}
	}
	return false
}

func newTestSessionManager(t *testing.T, action string, options StreamOptions) (*SessionManager, *lockedEventRecorder) {
	return newTestSessionManagerWithTimeout(t, action, options, 5*time.Second)
}

func newTestSessionManagerWithTimeout(t *testing.T, action string, options StreamOptions, timeout time.Duration) (*SessionManager, *lockedEventRecorder) {
	t.Helper()
	t.Setenv(sessionHelperEnvironment, "1")
	store, err := NewStore(testCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	executionOptions := execution.DefaultOptions(timeout)
	executionOptions.OutputLimit = options.OutputBudget
	executionOptions.ChunkBytes = options.ChunkBytes
	executionOptions.InterruptGrace = 25 * time.Millisecond
	executionOptions.TerminateGrace = 25 * time.Millisecond
	runner, err := execution.NewWithOptions(executionOptions)
	if err != nil {
		t.Fatal(err)
	}
	core := agent.NewCore(allowStage{}, allowStage{}, helperTranslator{action: action}, runner)
	recorder := &lockedEventRecorder{}
	handler, err := NewProfileHandlerWithSink(config.ProfileNetworkObserver, store, core, recorder)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewSessionManager(config.ProfileNetworkObserver, store, handler, recorder, options)
	if err != nil {
		t.Fatal(err)
	}
	return manager, recorder
}

func streamRequest(t *testing.T, user string) *contract.ExecutionRequest {
	t.Helper()
	request, err := contract.NewExecutionRequest("capture", map[string]string{"port": "53", "host": "127.0.0.1"}, contract.Metadata{BrokerType: "web", UserID: user})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

// TEST-STREAM-007..011
func TestSessionManagerStreamsAndTerminatesIdempotently(t *testing.T) {
	options := DefaultStreamOptions()
	options.ChunkBytes = 4
	options.OutputBudget = 1024
	options.IdleTimeout = time.Second
	options.HistoryTTL = time.Minute
	manager, recorder := newTestSessionManager(t, "interrupt", options)
	request := streamRequest(t, "alice")
	session, err := manager.StartStream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.ID) != 48 || session.ID == request.RequestID {
		t.Fatalf("invalid session ID %q", session.ID)
	}
	first := <-session.Events
	if first.Type != contract.FrameSessionStarted || first.State != contract.SessionRunning {
		t.Fatalf("first frame = %+v", first)
	}
	var stdout, stderr []byte
	var lastSequence uint64
	deadline := time.After(2 * time.Second)
	for len(stdout) < len("ready") || len(stderr) < len("diagnostic") {
		select {
		case frame := <-session.Events:
			if frame.Sequence > 0 {
				if frame.Sequence <= lastSequence {
					t.Fatalf("non-monotonic sequence: %d after %d", frame.Sequence, lastSequence)
				}
				lastSequence = frame.Sequence
			}
			if frame.Type == contract.FrameStdout {
				stdout = append(stdout, frame.Data...)
			} else if frame.Type == contract.FrameStderr {
				stderr = append(stderr, frame.Data...)
			}
		case <-deadline:
			t.Fatalf("initial output missing: stdout=%q stderr=%q", stdout, stderr)
		}
	}
	denied, err := manager.Terminate(context.Background(), contract.ControlRequest{Version: contract.StreamProtocolVersion, Action: contract.ControlTerminate, SessionID: session.ID, Owner: contract.SessionOwner{BrokerType: "web", UserID: "mallory"}})
	if err != nil || denied.Status != contract.ControlDenied {
		t.Fatalf("cross-owner terminate = %+v, %v", denied, err)
	}
	control := contract.ControlRequest{Version: contract.StreamProtocolVersion, Action: contract.ControlTerminate, SessionID: session.ID, Owner: contract.OwnerFromMetadata(request.Metadata)}
	result, err := manager.Terminate(context.Background(), control)
	if err != nil || result.Status != contract.ControlTerminating {
		t.Fatalf("terminate = %+v, %v", result, err)
	}
	repeated, err := manager.Terminate(context.Background(), control)
	if err != nil || (repeated.Status != contract.ControlAlreadyTerminating && repeated.Status != contract.ControlAlreadyExited) {
		t.Fatalf("repeated terminate = %+v, %v", repeated, err)
	}
	var final contract.StreamFrame
	for frame := range session.Events {
		if frame.Sequence <= lastSequence {
			t.Fatalf("non-monotonic sequence: %d after %d", frame.Sequence, lastSequence)
		}
		lastSequence = frame.Sequence
		if frame.Type == contract.FrameStdout {
			stdout = append(stdout, frame.Data...)
		}
		if frame.Type == contract.FrameSessionExited {
			final = frame
		}
	}
	if final.Response == nil || final.Response.Result != contract.ResultExecutionError || final.Response.Stdout != "" || final.Response.Stderr != "" {
		t.Fatalf("final frame = %+v", final)
	}
	late, err := manager.Terminate(context.Background(), control)
	if err != nil || late.Status != contract.ControlAlreadyExited {
		t.Fatalf("late terminate = %+v, %v", late, err)
	}
	unknown, err := manager.Terminate(context.Background(), contract.ControlRequest{Version: contract.StreamProtocolVersion, Action: contract.ControlTerminate, SessionID: "unknown", Owner: control.Owner})
	if err != nil || unknown.Status != contract.ControlNotFound {
		t.Fatalf("unknown terminate = %+v, %v", unknown, err)
	}
	for _, outcome := range []string{"session-started", "stream-opened", "terminate-requested", "interrupt-delivered", "session-exited"} {
		if !recorder.has(outcome) {
			t.Fatalf("missing audit outcome %q", outcome)
		}
	}
	recorder.mu.Lock()
	encoded, _ := json.Marshal(recorder.events)
	recorder.mu.Unlock()
	if string(encoded) == "" || containsBytes(encoded, []byte("ready")) || containsBytes(encoded, []byte("diagnostic")) {
		t.Fatalf("audit contains stream data: %s", encoded)
	}
}

func containsBytes(haystack, needle []byte) bool {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		match := true
		for offset := range needle {
			if haystack[index+offset] != needle[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TEST-STREAM-012
func TestSessionManagerBackpressureFailsClosed(t *testing.T) {
	options := DefaultStreamOptions()
	options.ChunkBytes = 8
	options.OutputBudget = 1 << 20
	options.QueueDepth = 2
	options.IdleTimeout = time.Second
	manager, recorder := newTestSessionManager(t, "flood", options)
	if _, err := manager.StartStream(context.Background(), streamRequest(t, "alice")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !recorder.has("backpressure-overflow") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !recorder.has("backpressure-overflow") {
		t.Fatal("backpressure overflow was not audited")
	}
}

// TEST-STREAM-019
func TestSessionManagerOutputBudgetFailsClosed(t *testing.T) {
	options := DefaultStreamOptions()
	options.ChunkBytes = 8
	options.OutputBudget = 64
	options.QueueDepth = 32
	options.IdleTimeout = time.Second
	manager, recorder := newTestSessionManager(t, "flood", options)
	session, err := manager.StartStream(context.Background(), streamRequest(t, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for frame := range session.Events {
		total += len(frame.Data)
	}
	if total > int(options.OutputBudget) || !recorder.has("output-limit") || !recorder.has("session-exited") {
		t.Fatalf("total=%d output-limit=%v session-exited=%v", total, recorder.has("output-limit"), recorder.has("session-exited"))
	}
}

// TEST-STREAM-021
func TestSessionManagerBoundsConcurrentSessions(t *testing.T) {
	options := DefaultStreamOptions()
	options.MaxSessions = 1
	options.IdleTimeout = time.Second
	options.HistoryTTL = time.Minute
	manager, _ := newTestSessionManager(t, "ignore", options)
	request := streamRequest(t, "alice")
	session, err := manager.StartStream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartStream(context.Background(), streamRequest(t, "bob")); err == nil {
		t.Fatal("concurrent session limit was not enforced")
	}
	control := contract.ControlRequest{Version: contract.StreamProtocolVersion, Action: contract.ControlTerminate, SessionID: session.ID, Owner: contract.OwnerFromMetadata(request.Metadata)}
	if _, err := manager.Terminate(context.Background(), control); err != nil {
		t.Fatal(err)
	}
	for range session.Events {
	}
}

// TEST-STREAM-023
func TestSessionManagerRequiresOwnerClaims(t *testing.T) {
	manager, _ := newTestSessionManager(t, "interrupt", DefaultStreamOptions())
	request, err := contract.NewExecutionRequest("capture", nil, contract.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartStream(context.Background(), request); err == nil {
		t.Fatal("streaming session without owner claims was accepted")
	}
}

// TEST-STREAM-024
func TestSessionExitAuditFailureReturnsExecutionError(t *testing.T) {
	options := DefaultStreamOptions()
	options.HistoryTTL = time.Minute
	manager, _ := newTestSessionManager(t, "success", options)
	manager.sink = failSessionExitSink{}
	session, err := manager.StartStream(context.Background(), streamRequest(t, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	var response *contract.ExecutionResponse
	for frame := range session.Events {
		if frame.Type == contract.FrameSessionExited {
			response = frame.Response
		}
	}
	if response == nil || response.Result != contract.ResultExecutionError || response.FailureStage != "execution" {
		t.Fatalf("final response = %+v", response)
	}
}

// TEST-STREAM-013
func TestSessionManagerIdleTimeoutFailsClosed(t *testing.T) {
	options := DefaultStreamOptions()
	options.IdleTimeout = 50 * time.Millisecond
	options.HistoryTTL = time.Minute
	manager, recorder := newTestSessionManager(t, "ignore", options)
	session, err := manager.StartStream(context.Background(), streamRequest(t, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	for range session.Events {
	}
	if !recorder.has("idle-timeout") || !recorder.has("session-exited") {
		t.Fatalf("idle lifecycle missing: %+v", recorder.events)
	}
}

type gatewayStreamingClient struct {
	starts     int
	terminates int
}

func (c *gatewayStreamingClient) Handle(_ context.Context, request *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	return &contract.ExecutionResponse{RequestID: request.RequestID, Result: contract.ResultSuccess}, nil
}

func (c *gatewayStreamingClient) StartStream(_ context.Context, request *contract.ExecutionRequest) (*StreamSession, error) {
	c.starts++
	id := "opaque-session"
	events := make(chan contract.StreamFrame, 1)
	events <- contract.StreamFrame{Version: contract.StreamProtocolVersion, Type: contract.FrameSessionStarted, RequestID: request.RequestID, SessionID: id, State: contract.SessionRunning, OccurredAt: time.Now().UTC()}
	close(events)
	return &StreamSession{ID: id, RequestID: request.RequestID, Events: events}, nil
}

func (c *gatewayStreamingClient) Terminate(_ context.Context, request contract.ControlRequest) (contract.ControlResult, error) {
	c.terminates++
	return contract.ControlResult{Version: contract.StreamProtocolVersion, SessionID: request.SessionID, Status: contract.ControlTerminating}, nil
}

// TEST-STREAM-015
func TestGatewayRoutesControlOnlyToOwningRunnerAndOwner(t *testing.T) {
	store, err := NewStore(testCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	network := &gatewayStreamingClient{}
	diagnostics := &gatewayStreamingClient{}
	gateway, err := NewGateway(store, map[config.RunnerProfile]Client{
		config.ProfileNetworkObserver: network,
		config.ProfileDiagnostics:     diagnostics,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := streamRequest(t, "alice")
	session, err := gateway.StartStream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if network.starts != 1 || diagnostics.starts != 0 || session.ID != "opaque-session" {
		t.Fatalf("routing starts network=%d diagnostics=%d session=%q", network.starts, diagnostics.starts, session.ID)
	}
	denied, err := gateway.Terminate(context.Background(), contract.ControlRequest{Version: contract.StreamProtocolVersion, Action: contract.ControlTerminate, SessionID: session.ID, Owner: contract.SessionOwner{BrokerType: "ssh", UserID: "mallory"}})
	if err != nil || denied.Status != contract.ControlDenied || network.terminates != 0 {
		t.Fatalf("denied result=%+v error=%v calls=%d", denied, err, network.terminates)
	}
	control := contract.ControlRequest{Version: contract.StreamProtocolVersion, Action: contract.ControlTerminate, SessionID: session.ID, Owner: contract.OwnerFromMetadata(request.Metadata)}
	result, err := gateway.Terminate(context.Background(), control)
	if err != nil || result.Status != contract.ControlTerminating || network.terminates != 1 || diagnostics.terminates != 0 {
		t.Fatalf("result=%+v error=%v network=%d diagnostics=%d", result, err, network.terminates, diagnostics.terminates)
	}
	short, _ := contract.NewExecutionRequest("system_info", nil, request.Metadata)
	if _, err := gateway.StartStream(context.Background(), short); err == nil {
		t.Fatal("short operation accepted by streaming API")
	}
}
