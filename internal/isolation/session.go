package isolation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"safeterm/internal/audit"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/execution"
)

var (
	ErrStreamBackpressure = errors.New("isolation: stream backpressure limit exceeded")
	ErrStreamOutputBudget = errors.New("isolation: stream output budget exceeded")
	ErrStreamIdleTimeout  = errors.New("isolation: stream idle timeout exceeded")
	ErrTerminateRequested = errors.New("isolation: semantic terminate requested")
	ErrClientDisconnected = errors.New("isolation: streaming client disconnected")
)

type StreamOptions struct {
	ChunkBytes   int
	OutputBudget int64
	QueueDepth   int
	MaxSessions  int
	IdleTimeout  time.Duration
	HistoryTTL   time.Duration
}

func DefaultStreamOptions() StreamOptions {
	return StreamOptions{
		ChunkBytes: 16 << 10, OutputBudget: 1 << 20, QueueDepth: 64, MaxSessions: 64,
		IdleTimeout: 5 * time.Minute, HistoryTTL: 5 * time.Minute,
	}
}

type StreamSession struct {
	ID        string
	RequestID string
	Events    <-chan contract.StreamFrame
}

type StreamingService interface {
	StartStream(context.Context, *contract.ExecutionRequest) (*StreamSession, error)
	Terminate(context.Context, contract.ControlRequest) (contract.ControlResult, error)
}

type SessionManager struct {
	profile  config.RunnerProfile
	catalog  *Store
	handler  Client
	sink     EventSink
	options  StreamOptions
	mu       sync.RWMutex
	sessions map[string]*sessionEntry
	active   int
}

type sessionEntry struct {
	expiry     *time.Timer
	manager    *SessionManager
	id         string
	request    *contract.ExecutionRequest
	owner      contract.SessionOwner
	version    string
	events     chan contract.StreamFrame
	cancel     context.CancelCauseFunc
	mu         sync.Mutex
	state      contract.SessionState
	sequence   uint64
	total      int64
	lastOutput time.Time
}

func NewSessionManager(profile config.RunnerProfile, catalog *Store, handler Client, sink EventSink, options StreamOptions) (*SessionManager, error) {
	if catalog == nil || catalog.Current() == nil || handler == nil {
		return nil, fmt.Errorf("isolation: session manager requires catalog and handler")
	}
	if _, ok := catalog.Current().Profile(profile); !ok {
		return nil, fmt.Errorf("isolation: unknown streaming profile %q", profile)
	}
	if options.ChunkBytes <= 0 || options.OutputBudget <= 0 || options.QueueDepth < 2 || options.MaxSessions <= 0 || options.IdleTimeout <= 0 || options.HistoryTTL <= 0 {
		return nil, fmt.Errorf("isolation: invalid stream limits")
	}
	if int64(options.ChunkBytes) > options.OutputBudget {
		return nil, fmt.Errorf("isolation: stream chunk exceeds output budget")
	}
	if options.ChunkBytes > contract.MaxStreamChunkBytes {
		return nil, fmt.Errorf("isolation: stream chunk exceeds protocol maximum")
	}
	return &SessionManager{
		profile: profile, catalog: catalog, handler: handler, sink: sink,
		options: options, sessions: map[string]*sessionEntry{},
	}, nil
}

func (m *SessionManager) StartStream(ctx context.Context, req *contract.ExecutionRequest) (*StreamSession, error) {
	if m == nil || ctx == nil || req == nil {
		return nil, fmt.Errorf("isolation: invalid streaming request")
	}
	snapshot := m.catalog.Current()
	operation, ok := snapshot.Operation(req.Command)
	if !ok || operation.RunnerProfile != m.profile {
		return nil, fmt.Errorf("isolation: operation %q is not assigned to profile %q", req.Command, m.profile)
	}
	if !operation.LongRunning {
		return nil, fmt.Errorf("isolation: operation %q is not long-running", req.Command)
	}
	owner := contract.OwnerFromMetadata(req.Metadata)
	if owner.BrokerType == "" || owner.UserID == "" {
		return nil, fmt.Errorf("isolation: streaming session requires broker and user ownership claims")
	}
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancelCause(ctx)
	entry := &sessionEntry{
		manager: m, id: id, request: req, owner: owner,
		version: snapshot.Version(), events: make(chan contract.StreamFrame, m.options.QueueDepth),
		cancel: cancel, state: contract.SessionStarting, lastOutput: time.Now().UTC(),
	}
	m.mu.Lock()
	if m.active >= m.options.MaxSessions {
		m.mu.Unlock()
		cancel(nil)
		return nil, fmt.Errorf("isolation: streaming session limit reached")
	}
	m.sessions[id] = entry
	m.active++
	m.mu.Unlock()
	release := func() {
		m.mu.Lock()
		delete(m.sessions, id)
		m.active--
		m.mu.Unlock()
	}
	if err := entry.audit(ctx, "session-started", nil); err != nil {
		cancel(err)
		release()
		return nil, err
	}
	if err := entry.audit(ctx, "stream-opened", nil); err != nil {
		cancel(err)
		release()
		return nil, err
	}

	if req.Metadata.BrokerType == "console" && !req.Metadata.SessionExpiresAt.IsZero() {
		entry.expiry = time.AfterFunc(time.Until(req.Metadata.SessionExpiresAt), func() { cancel(context.DeadlineExceeded) })
	}
	entry.state = contract.SessionRunning
	entry.events <- contract.StreamFrame{
		Version: contract.StreamProtocolVersion, Type: contract.FrameSessionStarted,
		RequestID: req.RequestID, SessionID: id, State: contract.SessionRunning,
		OccurredAt: time.Now().UTC(),
	}
	go entry.run(runCtx)
	go entry.watchIdle(runCtx)
	return &StreamSession{ID: id, RequestID: req.RequestID, Events: entry.events}, nil
}

func (m *SessionManager) Terminate(ctx context.Context, request contract.ControlRequest) (contract.ControlResult, error) {
	if m == nil || ctx == nil {
		return contract.ControlResult{}, fmt.Errorf("isolation: invalid terminate request")
	}
	if err := request.Validate(); err != nil {
		return contract.ControlResult{}, err
	}
	result := contract.ControlResult{Version: contract.StreamProtocolVersion, SessionID: request.SessionID}
	m.mu.RLock()
	entry := m.sessions[request.SessionID]
	m.mu.RUnlock()
	if entry == nil {
		result.Status = contract.ControlNotFound
		return result, nil
	}
	entry.mu.Lock()
	if entry.owner != request.Owner {
		entry.mu.Unlock()
		result.Status = contract.ControlDenied
		return result, nil
	}
	switch entry.state {
	case contract.SessionExited:
		entry.mu.Unlock()
		result.Status = contract.ControlAlreadyExited
		return result, nil
	case contract.SessionTerminating:
		entry.mu.Unlock()
		result.Status = contract.ControlAlreadyTerminating
		return result, nil
	}
	entry.state = contract.SessionTerminating
	entry.mu.Unlock()
	if err := entry.audit(ctx, "terminate-requested", nil); err != nil {
		entry.mu.Lock()
		if entry.state == contract.SessionTerminating {

			entry.state = contract.SessionRunning
		}
		entry.mu.Unlock()
		return contract.ControlResult{}, err
	}
	entry.mu.Lock()
	if entry.state == contract.SessionExited {
		entry.mu.Unlock()
		result.Status = contract.ControlAlreadyExited
		return result, nil
	}
	entry.mu.Unlock()
	entry.cancel(ErrTerminateRequested)
	result.Status = contract.ControlTerminating
	return result, nil
}

func (m *SessionManager) disconnect(ctx context.Context, sessionID string, owner contract.SessionOwner) {
	m.mu.RLock()
	entry := m.sessions[sessionID]
	m.mu.RUnlock()
	if entry == nil {
		return
	}
	entry.mu.Lock()
	if entry.owner != owner || entry.state == contract.SessionExited {
		entry.mu.Unlock()
		return
	}
	entry.state = contract.SessionTerminating
	entry.mu.Unlock()
	_ = entry.audit(ctx, "client-disconnected", nil)
	entry.cancel(ErrClientDisconnected)
}

func (s *sessionEntry) Emit(stream execution.Stream, data []byte) error {
	for len(data) > 0 {
		size := len(data)
		if size > s.manager.options.ChunkBytes {
			size = s.manager.options.ChunkBytes
		}
		part := append([]byte(nil), data[:size]...)
		s.mu.Lock()
		if s.state == contract.SessionExited {
			s.mu.Unlock()
			return context.Canceled
		}
		if s.total+int64(size) > s.manager.options.OutputBudget {
			s.mu.Unlock()
			_ = s.audit(context.Background(), "output-limit", nil)
			s.cancel(ErrStreamOutputBudget)
			return ErrStreamOutputBudget
		}
		s.sequence++
		sequence := s.sequence
		s.total += int64(size)
		s.lastOutput = time.Now().UTC()
		frameType := contract.FrameStdout
		if stream == execution.StreamStderr {
			frameType = contract.FrameStderr
		}
		frame := contract.StreamFrame{
			Version: contract.StreamProtocolVersion, Type: frameType,
			RequestID: s.request.RequestID, SessionID: s.id, Sequence: sequence,
			Data: part, State: contract.SessionRunning, OccurredAt: time.Now().UTC(),
		}
		select {
		case s.events <- frame:
			s.mu.Unlock()
		default:
			s.mu.Unlock()
			_ = s.audit(context.Background(), "backpressure-overflow", nil)
			s.cancel(ErrStreamBackpressure)
			return ErrStreamBackpressure
		}
		data = data[size:]
	}
	return nil
}

func (s *sessionEntry) RecordLifecycle(event string) {
	switch event {
	case execution.LifecycleInterruptDelivered, execution.LifecycleGraceExpired, execution.LifecycleForcedKill, execution.LifecycleOutputLimit:
		_ = s.audit(context.Background(), event, nil)
	}
}

func (s *sessionEntry) run(ctx context.Context) {
	defer s.cancel(nil)
	defer func() {
		if s.expiry != nil {
			s.expiry.Stop()
		}
	}()
	streamCtx := execution.WithLifecycleSink(execution.WithStreamSink(ctx, s), s)
	response, err := s.manager.handler.Handle(streamCtx, s.request)
	if err != nil || response == nil {
		now := time.Now().UTC()
		message := "streaming handler returned no response"
		if err != nil {
			message = err.Error()
		}
		response = &contract.ExecutionResponse{
			RequestID: s.request.RequestID, Result: contract.ResultExecutionError,
			FailureStage: "execution", Message: message, ExitCode: execution.ExitCodeUnavailable,
			StartedAt: now, EndedAt: now,
		}
	}
	final := *response
	final.Stdout, final.Stderr = "", ""
	s.mu.Lock()
	s.state = contract.SessionExited
	s.sequence++
	sequence := s.sequence
	s.mu.Unlock()
	if err := s.audit(context.Background(), "session-exited", &final); err != nil {
		final.Result = contract.ResultExecutionError
		final.FailureStage = "execution"
		final.Message = err.Error()
		final.ExitCode = execution.ExitCodeUnavailable
	}
	frame := contract.StreamFrame{
		Version: contract.StreamProtocolVersion, Type: contract.FrameSessionExited,
		RequestID: s.request.RequestID, SessionID: s.id, Sequence: sequence,
		State: contract.SessionExited, Response: &final, OccurredAt: time.Now().UTC(),
	}
	select {
	case s.events <- frame:
	case <-time.After(time.Second):
	}
	close(s.events)
	s.manager.mu.Lock()
	s.manager.active--
	s.manager.mu.Unlock()
	time.AfterFunc(s.manager.options.HistoryTTL, func() {
		s.manager.mu.Lock()
		delete(s.manager.sessions, s.id)
		s.manager.mu.Unlock()
	})
}

func (s *sessionEntry) watchIdle(ctx context.Context) {
	interval := s.manager.options.IdleTimeout / 2
	if interval > time.Second {
		interval = time.Second
	}
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			idle := now.Sub(s.lastOutput) >= s.manager.options.IdleTimeout
			exited := s.state == contract.SessionExited
			if idle && !exited {
				s.state = contract.SessionTerminating
			}
			s.mu.Unlock()
			if idle && !exited {
				_ = s.audit(context.Background(), "idle-timeout", nil)
				s.cancel(ErrStreamIdleTimeout)
				return
			}
		}
	}
}

func (s *sessionEntry) audit(ctx context.Context, outcome string, response *contract.ExecutionResponse) error {
	if s.manager.sink == nil {
		return nil
	}
	event := audit.NewSessionEvent(s.request, s.version, audit.ComponentRunner, string(s.manager.profile), s.id, outcome, response)
	if err := s.manager.sink.Record(ctx, event); err != nil {
		return fmt.Errorf("isolation: audit stream event: %w", err)
	}
	return nil
}

func generateSessionID() (string, error) {
	var data [24]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("isolation: generate session ID: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}
