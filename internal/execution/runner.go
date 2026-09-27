// Package execution implements the real process runner used by Agent Core.
// It executes a command with separate argv elements and never invokes a shell.
// See REQ-007 and ADR-008 for timeout and exit-code semantics.
package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// ExitCodeUnavailable means no portable process exit status exists because
// the process did not start, was canceled, or was terminated by a deadline.
const ExitCodeUnavailable = -1

// Runner implements agent.Executor with a mandatory per-process timeout.
type Runner struct {
	timeout time.Duration
}

// New constructs a Runner. An unlimited or already-expired timeout is rejected
// so a configuration mistake cannot silently create an unbounded process.
func New(timeout time.Duration) (*Runner, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("execution: timeout must be positive, got %s", timeout)
	}
	return &Runner{timeout: timeout}, nil
}

// Execute starts command directly with args as distinct argv elements. It
// captures both output streams and returns any bytes produced before failure.
func (r *Runner) Execute(ctx context.Context, command string, args []string) (stdout, stderr string, exitCode int, err error) {
	if ctx == nil {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: nil context")
	}
	if command == "" {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: command must not be empty")
	}
	if r == nil || r.timeout <= 0 {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: runner has no positive timeout")
	}

	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, command, args...)
	var stdoutBuffer, stderrBuffer bytes.Buffer
	cmd.Stdout = &stdoutBuffer
	cmd.Stderr = &stderrBuffer

	runErr := cmd.Run()
	stdout, stderr = stdoutBuffer.String(), stderrBuffer.String()
	if runErr == nil {
		return stdout, stderr, 0, nil
	}

	if contextErr := runCtx.Err(); contextErr != nil {
		if errors.Is(contextErr, context.DeadlineExceeded) {
			return stdout, stderr, ExitCodeUnavailable,
				fmt.Errorf("execution: command %q exceeded its deadline: %w", command, contextErr)
		}
		return stdout, stderr, ExitCodeUnavailable,
			fmt.Errorf("execution: command %q was canceled: %w", command, contextErr)
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return stdout, stderr, exitErr.ExitCode(),
			fmt.Errorf("execution: command %q exited with code %d: %w", command, exitErr.ExitCode(), runErr)
	}

	return stdout, stderr, ExitCodeUnavailable,
		fmt.Errorf("execution: command %q could not start: %w", command, runErr)
}
