//go:build linux

package execution

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"syscall"
	"testing"
	"time"
)

type testStreamSink struct {
	mu     sync.Mutex
	stdout bytes.Buffer
	stderr bytes.Buffer
	ready  chan struct{}
	once   sync.Once
}

func (s *testStreamSink) Emit(stream Stream, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stream == StreamStdout {
		s.stdout.Write(data)
		if bytes.Contains(s.stdout.Bytes(), []byte("ready")) && s.ready != nil {
			s.once.Do(func() { close(s.ready) })
		}
	} else {
		s.stderr.Write(data)
	}
	return nil
}

type testLifecycleSink struct {
	mu     sync.Mutex
	events []string
}

func (s *testLifecycleSink) RecordLifecycle(event string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *testLifecycleSink) contains(wanted string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range s.events {
		if event == wanted {
			return true
		}
	}
	return false
}

// TEST-STREAM-001, TEST-STREAM-003
func TestRunnerStreamsSeparateBoundedChunks(t *testing.T) {
	command, args := helperInvocation(t, "success")
	sink := &testStreamSink{}
	options := DefaultOptions(5 * time.Second)
	options.ChunkBytes = 4
	runner, err := NewWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithStreamSink(context.Background(), sink)
	stdout, stderr, code, err := runner.Execute(ctx, command, args)
	if err != nil || code != 0 || stdout != "stdout-success" || stderr != "stderr-success" {
		t.Fatalf("Execute() = (%q, %q, %d, %v)", stdout, stderr, code, err)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.stdout.String() != stdout || sink.stderr.String() != stderr {
		t.Fatalf("streamed stdout=%q stderr=%q", sink.stdout.String(), sink.stderr.String())
	}
}

// TEST-STREAM-004
func TestRunnerOutputBudgetCancelsProcess(t *testing.T) {
	command, args := helperInvocation(t, "flood")
	options := DefaultOptions(5 * time.Second)
	options.OutputLimit = 32
	options.ChunkBytes = 8
	runner, err := NewWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code, err := runner.Execute(context.Background(), command, args)
	if !errors.Is(err, ErrOutputLimit) || code != ExitCodeUnavailable || len(stdout)+len(stderr) != 32 {
		t.Fatalf("Execute() = (%d bytes, %d, %v)", len(stdout)+len(stderr), code, err)
	}
}

// TEST-STREAM-005
func TestRunnerCancellationUsesInterruptBeforeEscalation(t *testing.T) {
	command, args := helperInvocation(t, "interrupt")
	options := DefaultOptions(5 * time.Second)
	options.InterruptGrace = 300 * time.Millisecond
	options.TerminateGrace = 300 * time.Millisecond
	runner, err := NewWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	sink := &testStreamSink{ready: make(chan struct{})}
	lifecycle := &testLifecycleSink{}
	ctx, cancel := context.WithCancel(context.Background())
	ctx = WithLifecycleSink(WithStreamSink(ctx, sink), lifecycle)
	done := make(chan error, 1)
	go func() {
		_, _, _, err := runner.Execute(ctx, command, args)
		done <- err
	}()
	select {
	case <-sink.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("helper did not become ready")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	if !lifecycle.contains(LifecycleInterruptDelivered) || lifecycle.contains(LifecycleForcedKill) {
		t.Fatalf("lifecycle events = %#v", lifecycle.events)
	}
}

// TEST-STREAM-006
func TestRunnerEscalatesUnresponsiveProcessGroup(t *testing.T) {
	command, args := helperInvocation(t, "ignore")
	options := DefaultOptions(5 * time.Second)
	options.InterruptGrace = 30 * time.Millisecond
	options.TerminateGrace = 30 * time.Millisecond
	runner, err := NewWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	sink := &testStreamSink{ready: make(chan struct{})}
	lifecycle := &testLifecycleSink{}
	ctx, cancel := context.WithCancel(context.Background())
	ctx = WithLifecycleSink(WithStreamSink(ctx, sink), lifecycle)
	done := make(chan error, 1)
	go func() {
		_, _, _, err := runner.Execute(ctx, command, args)
		done <- err
	}()
	select {
	case <-sink.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("helper did not become ready")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	if !lifecycle.contains(LifecycleInterruptDelivered) || !lifecycle.contains(LifecycleForcedKill) {
		t.Fatalf("lifecycle events = %#v", lifecycle.events)
	}
}

// TEST-STREAM-017
func TestRunnerKillsDescendantsInDedicatedProcessGroup(t *testing.T) {
	command, args := helperInvocation(t, "spawn-child")
	options := DefaultOptions(5 * time.Second)
	options.InterruptGrace = 30 * time.Millisecond
	options.TerminateGrace = 30 * time.Millisecond
	runner, err := NewWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	sink := &testStreamSink{ready: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	ctx = WithStreamSink(ctx, sink)
	done := make(chan error, 1)
	go func() {
		_, _, _, err := runner.Execute(ctx, command, args)
		done <- err
	}()
	select {
	case <-sink.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("parent did not report child PID")
	}
	sink.mu.Lock()
	childPID, parseErr := parseHelperPID(sink.stdout.String())
	sink.mu.Unlock()
	if parseErr != nil || childPID <= 0 {
		t.Fatalf("child PID parse = %d, %v", childPID, parseErr)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(childPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("descendant process %d survived process-group termination", childPID)
}
