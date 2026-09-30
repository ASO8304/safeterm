//go:build linux

package integrity

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/execution"
)

type recordingExecutor struct {
	calls   int
	command string
	args    []string
	read    []byte
	before  func()
}

func (e *recordingExecutor) Execute(_ context.Context, command string, args []string) (string, string, int, error) {
	e.calls++
	e.command = command
	e.args = append([]string(nil), args...)
	if e.before != nil {
		e.before()
	}
	e.read, _ = os.ReadFile(command)
	return "ok", "", 0, nil
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}

func writeExecutable(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
}

func integrityConfig(path, checksum string) *config.Config {
	return &config.Config{
		Version: "test-v1",
		Profiles: []config.ProfileDefinition{{
			Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess,
			SocketPath: "/run/safeterm/test.sock", RunAsUser: "test", MemoryLimitMiB: 64, ProcessLimit: 8,
		}},
		Executables: []config.ExecutableDefinition{{
			ID: "tool", Path: path, SHA256: checksum,
			AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics},
		}},
		Operations: []config.OperationDefinition{{
			Name: "probe", RunnerProfile: config.ProfileDiagnostics,
			ExecutorKind: config.ExecutorProcess, ExecutableRef: "tool",
		}},
	}
}

// TEST-INTEGRITY-001, TEST-INTEGRITY-005
func TestExecutorVerifiesAndExecutesPinnedOpenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	original := []byte("\x7fELFapproved executable bytes")
	replacement := []byte("\x7fELFreplacement executable")
	writeExecutable(t, path, original)

	delegate := &recordingExecutor{}
	delegate.before = func() {
		newPath := filepath.Join(dir, "replacement")
		writeExecutable(t, newPath, replacement)
		if err := os.Rename(newPath, path); err != nil {
			t.Fatal(err)
		}
	}
	executor, err := NewExecutor(integrityConfig(path, digest(original)), config.ProfileDiagnostics, delegate)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exitCode, err := executor.Execute(context.Background(), path, []string{"one", "two"})
	if err != nil || stdout != "ok" || stderr != "" || exitCode != 0 {
		t.Fatalf("Execute() = %q, %q, %d, %v", stdout, stderr, exitCode, err)
	}
	if delegate.calls != 1 || !strings.HasPrefix(delegate.command, "/proc/self/fd/") || strings.Join(delegate.args, ",") != "one,two" {
		t.Fatalf("delegate = %+v", delegate)
	}
	if string(delegate.read) != string(original) {
		t.Fatalf("executed bytes = %q, want original %q", delegate.read, original)
	}
}

// TEST-INTEGRITY-002
func TestNewExecutorFailsClosedOnStartupMismatchAndInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	writeExecutable(t, path, []byte("\x7fELFactual"))
	delegate := &recordingExecutor{}

	if _, err := NewExecutor(integrityConfig(path, digest([]byte("\x7fELFexpected"))), config.ProfileDiagnostics, delegate); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("mismatch error = %v", err)
	}
	if _, err := NewExecutor(integrityConfig(filepath.Join(dir, "missing"), digest(nil)), config.ProfileDiagnostics, delegate); err == nil {
		t.Fatal("missing executable accepted")
	}
	directory := filepath.Join(dir, "directory")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewExecutor(integrityConfig(directory, digest(nil)), config.ProfileDiagnostics, delegate); err == nil || !strings.Contains(err.Error(), "regular executable") {
		t.Fatalf("directory error = %v", err)
	}
	script := filepath.Join(dir, "script")
	scriptContent := []byte("#!/bin/sh\necho unsafe interpreter\n")
	writeExecutable(t, script, scriptContent)
	if _, err := NewExecutor(integrityConfig(script, digest(scriptContent)), config.ProfileDiagnostics, delegate); err == nil || !strings.Contains(err.Error(), "native ELF") {
		t.Fatalf("script error = %v", err)
	}
	writable := filepath.Join(dir, "writable")
	writableContent := []byte("\x7fELFwritable")
	writeExecutable(t, writable, writableContent)
	if err := os.Chmod(writable, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := NewExecutor(integrityConfig(writable, digest(writableContent)), config.ProfileDiagnostics, delegate); err == nil || !strings.Contains(err.Error(), "writable") {
		t.Fatalf("writable error = %v", err)
	}
}

// TEST-INTEGRITY-003, TEST-INTEGRITY-006
func TestExecuteRejectsContentChangeAndUnapprovedPathBeforeDelegate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	original := []byte("\x7fELFapproved")
	writeExecutable(t, path, original)
	delegate := &recordingExecutor{}
	executor, err := NewExecutor(integrityConfig(path, digest(original)), config.ProfileDiagnostics, delegate)
	if err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, path, []byte("\x7fELFtampered"))
	for _, command := range []string{path, "/bin/echo"} {
		stdout, stderr, exitCode, err := executor.Execute(context.Background(), command, nil)
		if err == nil || stdout != "" || stderr != "" || exitCode != exitCodeUnavailable {
			t.Fatalf("Execute(%q) = %q, %q, %d, %v", command, stdout, stderr, exitCode, err)
		}
	}
	if delegate.calls != 0 {
		t.Fatalf("delegate called %d times", delegate.calls)
	}
}

// TEST-INTEGRITY-004
func TestExecuteRejectsSymlinkRetargeting(t *testing.T) {
	dir := t.TempDir()
	targetOne := filepath.Join(dir, "one")
	targetTwo := filepath.Join(dir, "two")
	link := filepath.Join(dir, "tool")
	content := []byte("\x7fELFsame approved content")
	writeExecutable(t, targetOne, content)
	writeExecutable(t, targetTwo, content)
	if err := os.Symlink(targetOne, link); err != nil {
		t.Fatal(err)
	}
	delegate := &recordingExecutor{}
	executor, err := NewExecutor(integrityConfig(link, digest(content)), config.ProfileDiagnostics, delegate)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetTwo, link); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := executor.Execute(context.Background(), link, nil); err == nil || !strings.Contains(err.Error(), "canonical path changed") {
		t.Fatalf("retarget error = %v", err)
	}
	if delegate.calls != 0 {
		t.Fatal("retargeted symlink reached delegate")
	}
}

// TEST-INTEGRITY-006
func TestExecutorRejectsInvalidConstructionAndCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	content := []byte("\x7fELFapproved")
	writeExecutable(t, path, content)
	cfg := integrityConfig(path, digest(content))
	if _, err := NewExecutor(cfg, config.ProfileDiagnostics, nil); err == nil {
		t.Fatal("nil delegate accepted")
	}
	if _, err := NewExecutor(cfg, "ghost", &recordingExecutor{}); err == nil {
		t.Fatal("unknown profile accepted")
	}
	if _, _, _, err := (*Executor)(nil).Execute(context.Background(), path, nil); err == nil {
		t.Fatal("nil executor accepted")
	}
	executor, _ := NewExecutor(cfg, config.ProfileDiagnostics, &recordingExecutor{})
	if _, _, _, err := executor.Execute(nil, path, nil); err == nil {
		t.Fatal("nil context accepted")
	}
}

// TEST-INTEGRITY-013
func TestSystemctlControllerUsesVerifiedControllerBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "systemctl")
	content := []byte("\x7fELFapproved controller")
	writeExecutable(t, path, content)
	cfg := &config.Config{
		Version: "test-v1",
		Profiles: []config.ProfileDefinition{{
			Name: config.ProfileServiceManager, ExecutorKind: config.ExecutorSystemd,
			ControllerExecutableRef: "systemctl", SocketPath: "/run/safeterm/systemd.sock",
			RunAsUser: "test", MemoryLimitMiB: 64, ProcessLimit: 8,
		}},
		Executables: []config.ExecutableDefinition{{
			ID: "systemctl", Path: path, SHA256: digest(content),
			AllowedProfiles: []config.RunnerProfile{config.ProfileServiceManager},
		}},
	}
	delegate := &recordingExecutor{}
	verified, err := NewExecutor(cfg, config.ProfileServiceManager, delegate)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := execution.NewSystemctlController(path, verified)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := controller.Control(context.Background(), "restart", "demo.service"); err != nil {
		t.Fatal(err)
	}
	if delegate.calls != 1 || !strings.HasPrefix(delegate.command, "/proc/self/fd/") || strings.Join(delegate.args, ",") != "restart,demo.service" {
		t.Fatalf("controller delegate = %+v", delegate)
	}
}
