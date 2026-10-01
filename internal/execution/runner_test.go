package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"safeterm/internal/agent"
)

var _ agent.Executor = (*Runner)(nil)

const helperEnvironment = "SAFETERM_EXECUTION_HELPER"

func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv(helperEnvironment) != "1" {
		return
	}
	separator := -1
	for i, value := range os.Args {
		if value == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(97)
	}
	action := os.Args[separator+1]
	values := os.Args[separator+2:]
	switch action {
	case "success":
		fmt.Fprint(os.Stdout, "stdout-success")
		fmt.Fprint(os.Stderr, "stderr-success")
		os.Exit(0)
	case "exit":
		fmt.Fprint(os.Stdout, "stdout-before-exit")
		fmt.Fprint(os.Stderr, "stderr-before-exit")
		os.Exit(23)
	case "wait":
		fmt.Fprint(os.Stdout, "stdout-before-wait")
		fmt.Fprint(os.Stderr, "stderr-before-wait")
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "interrupt":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt)
		fmt.Fprint(os.Stdout, "ready")
		<-signals
		fmt.Fprint(os.Stdout, "interrupted")
		os.Exit(0)
	case "ignore":
		signals := make(chan os.Signal, 4)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		go func() {
			for range signals {
			}
		}()
		fmt.Fprint(os.Stdout, "ready")
		for {
			time.Sleep(time.Hour)
		}
	case "flood":
		for i := 0; i < 1024; i++ {
			fmt.Fprint(os.Stdout, "0123456789abcdef")
		}
		os.Exit(0)
	case "child-ignore":
		signals := make(chan os.Signal, 4)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		go func() {
			for range signals {
			}
		}()
		for {
			time.Sleep(time.Hour)
		}
	case "spawn-child":
		signals := make(chan os.Signal, 4)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		go func() {
			for range signals {
			}
		}()
		child := exec.Command(os.Args[0], "-test.run=^TestRunnerHelperProcess$", "--", "child-ignore")
		child.Env = os.Environ()
		if err := child.Start(); err != nil {
			os.Exit(94)
		}
		fmt.Fprintf(os.Stdout, "ready:%d", child.Process.Pid)
		for {
			time.Sleep(time.Hour)
		}
	case "args":
		if err := json.NewEncoder(os.Stdout).Encode(values); err != nil {
			os.Exit(96)
		}
		os.Exit(0)
	default:
		os.Exit(95)
	}
}

func parseHelperPID(value string) (int, error) {
	return strconv.Atoi(strings.TrimPrefix(value, "ready:"))
}

func helperInvocation(t *testing.T, action string, values ...string) (string, []string) {
	t.Helper()
	t.Setenv(helperEnvironment, "1")
	args := []string{"-test.run=^TestRunnerHelperProcess$", "--", action}
	return os.Args[0], append(args, values...)
}

func mustRunner(t *testing.T, timeout time.Duration) *Runner {
	t.Helper()
	runner, err := New(timeout)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

// TEST-EXEC-001
// Requirement: REQ-007 (AC-1)
func TestExecute_SuccessCapturesSeparateStreams(t *testing.T) {
	command, args := helperInvocation(t, "success")
	stdout, stderr, exitCode, err := mustRunner(t, 10*time.Second).Execute(context.Background(), command, args)
	if err != nil || stdout != "stdout-success" || stderr != "stderr-success" || exitCode != 0 {
		t.Fatalf("Execute() = (%q, %q, %d, %v)", stdout, stderr, exitCode, err)
	}
}

// TEST-EXEC-002
// Requirement: REQ-007 (AC-2)
func TestExecute_NonZeroExitPreservesOutputAndCode(t *testing.T) {
	command, args := helperInvocation(t, "exit")
	stdout, stderr, exitCode, err := mustRunner(t, 10*time.Second).Execute(context.Background(), command, args)
	if err == nil || stdout != "stdout-before-exit" || stderr != "stderr-before-exit" || exitCode != 23 {
		t.Fatalf("Execute() = (%q, %q, %d, %v)", stdout, stderr, exitCode, err)
	}
}

// TEST-EXEC-003
// Requirement: REQ-007 (AC-3)
func TestExecute_CommandCannotStartReturnsUnavailableExitCode(t *testing.T) {
	command := "safeterm-command-that-must-not-exist"
	if runtime.GOOS == "windows" {
		command += ".exe"
	}
	stdout, stderr, exitCode, err := mustRunner(t, 10*time.Second).Execute(context.Background(), command, nil)
	if err == nil || stdout != "" || stderr != "" || exitCode != ExitCodeUnavailable {
		t.Fatalf("Execute() = (%q, %q, %d, %v)", stdout, stderr, exitCode, err)
	}
}

// TEST-EXEC-004
// Requirement: REQ-007 (AC-4)
func TestExecute_TimeoutTerminatesAndPreservesOutput(t *testing.T) {
	command, args := helperInvocation(t, "wait")
	started := time.Now()
	stdout, stderr, exitCode, err := mustRunner(t, 100*time.Millisecond).Execute(context.Background(), command, args)
	if !errors.Is(err, context.DeadlineExceeded) || exitCode != ExitCodeUnavailable {
		t.Fatalf("expected deadline error and unavailable exit code, got (%d, %v)", exitCode, err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("timed-out process returned too slowly: %v", time.Since(started))
	}
	if stdout != "stdout-before-wait" || stderr != "stderr-before-wait" {
		t.Fatalf("partial output lost: stdout=%q stderr=%q", stdout, stderr)
	}
}

// TEST-EXEC-005
// Requirement: REQ-007 (AC-5)
func TestExecute_ParentCancellationTerminatesProcess(t *testing.T) {
	command, args := helperInvocation(t, "wait")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stdout, stderr, exitCode, err := mustRunner(t, 10*time.Second).Execute(ctx, command, args)
	if !errors.Is(err, context.Canceled) || exitCode != ExitCodeUnavailable || stdout != "" || stderr != "" {
		t.Fatalf("Execute() = (%q, %q, %d, %v)", stdout, stderr, exitCode, err)
	}
}

// TEST-EXEC-006
// Requirement: REQ-007 (AC-6)
func TestExecute_ArgumentsRemainExactArgvElements(t *testing.T) {
	values := []string{"two words", "", `"quoted"`, "$(whoami)", "; echo injected", "*", "line1\nline2"}
	command, args := helperInvocation(t, "args", values...)
	stdout, stderr, exitCode, err := mustRunner(t, 10*time.Second).Execute(context.Background(), command, args)
	if err != nil || stderr != "" || exitCode != 0 {
		t.Fatalf("Execute() = (%q, %q, %d, %v)", stdout, stderr, exitCode, err)
	}
	var got []string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode helper output %q: %v", stdout, err)
	}
	if !reflect.DeepEqual(got, values) {
		t.Fatalf("argv changed: got %#v, want %#v", got, values)
	}
}

// TEST-EXEC-007
// Requirement: REQ-007 (AC-7)
func TestRunner_InvalidConstructionAndCallsFailSafely(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		if runner, err := New(timeout); runner != nil || err == nil {
			t.Fatalf("New(%s) = (%v, %v), want nil and error", timeout, runner, err)
		}
	}
	runner := mustRunner(t, 10*time.Second)
	for _, tc := range []struct {
		name    string
		runner  *Runner
		ctx     context.Context
		command string
		message string
	}{
		{"nil context", runner, nil, os.Args[0], "nil context"},
		{"empty command", runner, context.Background(), "", "must not be empty"},
		{"nil runner", nil, context.Background(), os.Args[0], "no positive timeout"},
		{"zero runner", &Runner{}, context.Background(), os.Args[0], "no positive timeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode, err := tc.runner.Execute(tc.ctx, tc.command, nil)
			if err == nil || !strings.Contains(err.Error(), tc.message) || stdout != "" || stderr != "" || exitCode != ExitCodeUnavailable {
				t.Fatalf("Execute() = (%q, %q, %d, %v)", stdout, stderr, exitCode, err)
			}
		})
	}
}
