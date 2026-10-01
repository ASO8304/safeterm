// Package execution implements direct, bounded process execution without a
// shell. M11 adds optional bounded stream sinks and graceful process-group
// termination while preserving the agent.Executor contract.
package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

const ExitCodeUnavailable = -1

var ErrOutputLimit = errors.New("execution: output limit exceeded")

type Options struct {
	Timeout        time.Duration
	OutputLimit    int64
	ChunkBytes     int
	InterruptGrace time.Duration
	TerminateGrace time.Duration
}

func DefaultOptions(timeout time.Duration) Options {
	return Options{
		Timeout: timeout, OutputLimit: 1 << 20, ChunkBytes: 16 << 10,
		InterruptGrace: 500 * time.Millisecond, TerminateGrace: 500 * time.Millisecond,
	}
}

type Runner struct {
	options Options
}

func New(timeout time.Duration) (*Runner, error) {
	return NewWithOptions(DefaultOptions(timeout))
}

func NewWithOptions(options Options) (*Runner, error) {
	if options.Timeout <= 0 || options.OutputLimit <= 0 || options.ChunkBytes <= 0 ||
		options.InterruptGrace <= 0 || options.TerminateGrace <= 0 {
		return nil, fmt.Errorf("execution: timeout, output limit, chunk size, and grace periods must be positive")
	}
	if int64(options.ChunkBytes) > options.OutputLimit {
		return nil, fmt.Errorf("execution: chunk size cannot exceed output limit")
	}
	return &Runner{options: options}, nil
}

func (r *Runner) Execute(ctx context.Context, command string, args []string) (stdout, stderr string, exitCode int, err error) {
	if ctx == nil {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: nil context")
	}
	if command == "" {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: command must not be empty")
	}
	if r == nil || r.options.Timeout <= 0 || r.options.OutputLimit <= 0 || r.options.ChunkBytes <= 0 {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: runner has no positive timeout or bounded output configuration")
	}
	if err := ctx.Err(); err != nil {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: command %q was canceled: %w", command, err)
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	timer := time.AfterFunc(r.options.Timeout, func() { cancel(context.DeadlineExceeded) })
	defer timer.Stop()

	cmd := exec.Command(command, args...)
	configureProcessGroup(cmd)
	budget := &captureBudget{remaining: r.options.OutputLimit, cancel: cancel}
	var stdoutBuffer, stderrBuffer bytes.Buffer
	stdoutWriter := io.Writer(&captureWriter{budget: budget, buffer: &stdoutBuffer})
	stderrWriter := io.Writer(&captureWriter{budget: budget, buffer: &stderrBuffer})
	if sink := streamSinkFrom(ctx); sink != nil {
		stdoutWriter = io.MultiWriter(stdoutWriter, streamWriter(sink, StreamStdout, r.options.ChunkBytes))
		stderrWriter = io.MultiWriter(stderrWriter, streamWriter(sink, StreamStderr, r.options.ChunkBytes))
	}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	if startErr := cmd.Start(); startErr != nil {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: command %q could not start: %w", command, startErr)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()

	var runErr error
	select {
	case runErr = <-waited:
	case <-runCtx.Done():
		runErr = r.stopProcessGroup(ctx, cmd, waited)
	}
	stdout, stderr = stdoutBuffer.String(), stderrBuffer.String()

	if cause := context.Cause(runCtx); cause != nil {
		if errors.Is(cause, context.DeadlineExceeded) {
			return stdout, stderr, ExitCodeUnavailable, fmt.Errorf("execution: command %q exceeded its deadline: %w", command, cause)
		}
		if errors.Is(cause, ErrOutputLimit) {
			recordLifecycle(ctx, LifecycleOutputLimit)
			return stdout, stderr, ExitCodeUnavailable, fmt.Errorf("execution: command %q exceeded output limit: %w", command, cause)
		}
		return stdout, stderr, ExitCodeUnavailable, fmt.Errorf("execution: command %q was canceled: %w", command, cause)
	}
	if runErr == nil {
		return stdout, stderr, 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return stdout, stderr, exitErr.ExitCode(), fmt.Errorf("execution: command %q exited with code %d: %w", command, exitErr.ExitCode(), runErr)
	}
	return stdout, stderr, ExitCodeUnavailable, fmt.Errorf("execution: command %q failed: %w", command, runErr)
}

func (r *Runner) stopProcessGroup(ctx context.Context, cmd *exec.Cmd, waited <-chan error) error {
	if err := interruptProcessGroup(cmd.Process); err == nil {
		recordLifecycle(ctx, LifecycleInterruptDelivered)
	}
	select {
	case err := <-waited:
		return err
	case <-time.After(r.options.InterruptGrace):
		recordLifecycle(ctx, LifecycleGraceExpired)
	}
	_ = terminateProcessGroup(cmd.Process)
	select {
	case err := <-waited:
		return err
	case <-time.After(r.options.TerminateGrace):
		recordLifecycle(ctx, LifecycleGraceExpired)
	}
	if err := killProcessGroup(cmd.Process); err == nil {
		recordLifecycle(ctx, LifecycleForcedKill)
	}
	return <-waited
}

type captureBudget struct {
	mu        sync.Mutex
	remaining int64
	cancel    context.CancelCauseFunc
}

type captureWriter struct {
	budget *captureBudget
	buffer *bytes.Buffer
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.budget.mu.Lock()
	defer w.budget.mu.Unlock()
	if w.budget.remaining <= 0 {
		w.budget.cancel(ErrOutputLimit)
		return 0, ErrOutputLimit
	}
	accepted := len(p)
	if int64(accepted) > w.budget.remaining {
		accepted = int(w.budget.remaining)
	}
	_, _ = w.buffer.Write(p[:accepted])
	w.budget.remaining -= int64(accepted)
	if accepted != len(p) {
		w.budget.cancel(ErrOutputLimit)
		return accepted, ErrOutputLimit
	}
	return accepted, nil
}
