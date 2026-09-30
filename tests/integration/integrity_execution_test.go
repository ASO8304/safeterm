//go:build linux

package integration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/execution"
	"safeterm/internal/integrity"
	"safeterm/internal/policy"
	"safeterm/internal/translation"
	"safeterm/internal/validation"
)

func copyExecutable(t *testing.T) (string, string) {
	t.Helper()
	source, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	path := filepath.Join(t.TempDir(), "approved-test-helper")
	target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(target, hasher), source); err != nil {
		t.Fatal(err)
	}
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
	return path, fmt.Sprintf("%x", hasher.Sum(nil))
}

func integrityCore(t *testing.T, executablePath, checksum string) *agent.Core {
	t.Helper()
	t.Setenv(executionIntegrationEnvironment, "1")
	cfg := executionIntegrationConfig()
	cfg.Executables[0].Path = executablePath
	cfg.Executables[0].SHA256 = checksum
	translator, err := translation.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := execution.New(10 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := integrity.NewExecutor(cfg, config.ProfileDiagnostics, runner)
	if err != nil {
		t.Fatal(err)
	}
	return agent.NewCore(validation.New(cfg), policy.New(cfg), translator, verified)
}

// TEST-INTEGRITY-007
func TestCoreExecutesOnlyHashVerifiedConfiguredBinary(t *testing.T) {
	path, checksum := copyExecutable(t)
	core := integrityCore(t, path, checksum)
	request := executionRequest(t, "probe_success", []string{"operator"}, nil)
	response := core.HandleRequest(context.Background(), request)
	if response.Result != contract.ResultSuccess || response.Stdout != "integration-stdout" || integrationHelperStderr(response.Stderr) != "integration-stderr" || response.ExitCode != 0 {
		t.Fatalf("unexpected verified response: %+v", response)
	}
}

// TEST-INTEGRITY-008
func TestCoreReportsIntegrityChangeAsExecutionFailure(t *testing.T) {
	path, checksum := copyExecutable(t)
	core := integrityCore(t, path, checksum)
	if err := os.WriteFile(path, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	request := executionRequest(t, "probe_success", []string{"operator"}, nil)
	response := core.HandleRequest(context.Background(), request)
	if response.Result != contract.ResultExecutionError || response.FailureStage != "execution" || response.ExitCode != execution.ExitCodeUnavailable || response.Stdout != "" || response.Stderr != "" || !strings.Contains(response.Message, "integrity") {
		t.Fatalf("unexpected integrity response: %+v", response)
	}
}
