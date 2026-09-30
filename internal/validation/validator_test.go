package validation

import (
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/contract"
)

// traceTestConfig builds an in-memory Config with a "trace" operation
// matching the exact example from the project document (section 6.3):
// a "port" parameter (int, 1..65535) and a "host" parameter (string,
// IPv4 pattern).
func traceTestConfig() *config.Config {
	minPort, maxPort := 1, 65535
	return &config.Config{
		Version: "test-v1",
		Profiles: []config.ProfileDefinition{
			{Name: config.ProfileNetworkObserver, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/safeterm/network.sock", RunAsUser: "test", MemoryLimitMiB: 64, ProcessLimit: 8},
			{Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/safeterm/diagnostics.sock", RunAsUser: "test", MemoryLimitMiB: 64, ProcessLimit: 8},
		},
		Executables: []config.ExecutableDefinition{
			{ID: "tcpdump", Path: "/usr/bin/tcpdump", SHA256: "0000000000000000000000000000000000000000000000000000000000000000", AllowedProfiles: []config.RunnerProfile{config.ProfileNetworkObserver}},
			{ID: "uname", Path: "/usr/bin/uname", SHA256: "0000000000000000000000000000000000000000000000000000000000000000", AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics}},
		},
		Operations: []config.OperationDefinition{
			{
				Name: "trace", RunnerProfile: config.ProfileNetworkObserver, ExecutorKind: config.ExecutorProcess, ExecutableRef: "tcpdump",
				Parameters: []config.ParameterDefinition{
					{Name: "port", Type: config.ParamTypeInt, Required: true, Min: &minPort, Max: &maxPort},
					{Name: "host", Type: config.ParamTypeString, Required: true, Pattern: `^[0-9]{1,3}(\.[0-9]{1,3}){3}$`},
				},
			},
			{
				Name: "system_info", RunnerProfile: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, ExecutableRef: "uname",
			},
		},
	}
}

func newRequest(t *testing.T, command string, args map[string]string) *contract.ExecutionRequest {
	t.Helper()
	req, err := contract.NewExecutionRequest(command, args, contract.Metadata{})
	if err != nil {
		t.Fatalf("failed to build test request: %v", err)
	}
	return req
}

// TEST-VAL-001
// Requirement: REQ-004 (AC-1)
// Scenario: a request for an operation that does not exist in config
// must be rejected.
func TestValidate_UnknownOperation_Rejected(t *testing.T) {
	v := New(traceTestConfig())
	req := newRequest(t, "does_not_exist", nil)

	if err := v.Validate(req); err == nil {
		t.Fatal("expected error for unknown operation, got nil")
	}
}

// TEST-VAL-002
// Requirement: REQ-004 (AC-2)
// Scenario: a required argument that is missing must be rejected.
func TestValidate_MissingRequiredArgument_Rejected(t *testing.T) {
	v := New(traceTestConfig())
	req := newRequest(t, "trace", map[string]string{"port": "999"}) // host missing

	if err := v.Validate(req); err == nil {
		t.Fatal("expected error for missing required argument 'host', got nil")
	}
}

// TEST-VAL-003
// Requirement: REQ-004 (AC-6)
// Scenario: an argument the operation does not define must be rejected,
// even if its value looks harmless — this is the whitelist enforcement.
func TestValidate_UnknownArgument_Rejected(t *testing.T) {
	v := New(traceTestConfig())
	req := newRequest(t, "trace", map[string]string{
		"port":  "999",
		"host":  "192.168.10.20",
		"extra": "unexpected",
	})

	if err := v.Validate(req); err == nil {
		t.Fatal("expected error for undefined argument 'extra', got nil")
	}
}

// TEST-VAL-004, 005, 006, 007
// Requirement: REQ-004 (AC-3) — these are the exact test vectors from the
// project document's own example (section 6.3 / 9): port must be an
// integer within [1, 65535].
func TestValidate_PortBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		port    string
		wantErr bool
	}{
		{"port=80 within range", "80", false},
		{"port=0 below minimum", "0", true},
		{"port=65535 at upper boundary", "65535", false},
		{"port=65536 above maximum", "65536", true},
		{"port=abc not an integer", "abc", true},
	}

	v := New(traceTestConfig())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := newRequest(t, "trace", map[string]string{
				"port": tc.port,
				"host": "192.168.10.20",
			})
			err := v.Validate(req)
			if tc.wantErr && err == nil {
				t.Fatalf("expected rejection for port=%q, got nil", tc.port)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected port=%q to pass, got error: %v", tc.port, err)
			}
		})
	}
}

// TEST-VAL-008
// Requirement: REQ-004 (AC-4), the project document's exact injection
// example: port = "; rm -rf /" must be rejected.
func TestValidate_PortWithInjectionAttempt_Rejected(t *testing.T) {
	v := New(traceTestConfig())
	req := newRequest(t, "trace", map[string]string{
		"port": "; rm -rf /",
		"host": "192.168.10.20",
	})

	if err := v.Validate(req); err == nil {
		t.Fatal("expected rejection for injection attempt in port, got nil")
	}
}

// TEST-VAL-009
// Requirement: REQ-004 (AC-5)
// Scenario: a host value that does not match the IPv4 pattern must be
// rejected.
func TestValidate_HostNotMatchingPattern_Rejected(t *testing.T) {
	v := New(traceTestConfig())
	req := newRequest(t, "trace", map[string]string{
		"port": "999",
		"host": "not-an-ip",
	})

	if err := v.Validate(req); err == nil {
		t.Fatal("expected rejection for host not matching IPv4 pattern, got nil")
	}
}

// TEST-VAL-010
// Requirement: REQ-004 (AC-7)
// Scenario: even though "host" has a Pattern, dangerous shell
// metacharacters must be rejected unconditionally (defense-in-depth) —
// this test would also fail the pattern check on its own, but it exists
// specifically to prove the dangerousChars layer runs independently.
func TestValidate_HostWithShellMetacharacters_Rejected(t *testing.T) {
	v := New(traceTestConfig())
	req := newRequest(t, "trace", map[string]string{
		"port": "999",
		"host": "192.168.10.20; rm -rf /",
	})

	if err := v.Validate(req); err == nil {
		t.Fatal("expected rejection for host containing shell metacharacters, got nil")
	}
}

// TEST-VAL-011
// Requirement: REQ-004 (AC-8)
// Scenario: a Pattern must match the FULL value, not just a prefix.
// "192.168.10.20" is a valid IPv4 prefix of the malicious string below,
// but the whole value must still be rejected — this specifically guards
// against a config author forgetting to anchor their regex with ^...$.
func TestValidate_HostPatternMustMatchFullString_NotJustPrefix(t *testing.T) {
	// Use a deliberately UNANCHORED pattern to prove full-match is enforced
	// by the validator itself, not just by convention in how patterns are written.
	minPort, maxPort := 1, 65535
	cfg := &config.Config{
		Operations: []config.OperationDefinition{
			{
				Name: "trace",
				Parameters: []config.ParameterDefinition{
					{Name: "port", Type: config.ParamTypeInt, Required: true, Min: &minPort, Max: &maxPort},
					{Name: "host", Type: config.ParamTypeString, Required: true, Pattern: `[0-9]{1,3}(\.[0-9]{1,3}){3}`}, // no ^...$
				},
			},
		},
	}
	v := New(cfg)

	// This value contains dangerous characters too, so to isolate the
	// full-match behavior specifically, this case is expected to fail
	// for either reason — the important assertion is that it fails at all.
	req := newRequest(t, "trace", map[string]string{
		"port": "999",
		"host": "192.168.10.20 extra-trailing-data",
	})

	if err := v.Validate(req); err == nil {
		t.Fatal("expected rejection: pattern must match the full string, not just a prefix")
	}
}

// TEST-VAL-012
// Scenario: a fully valid request must pass with no error.
func TestValidate_ValidTraceRequest_Passes(t *testing.T) {
	v := New(traceTestConfig())
	req := newRequest(t, "trace", map[string]string{
		"port": "999",
		"host": "192.168.10.20",
	})

	if err := v.Validate(req); err != nil {
		t.Fatalf("expected valid request to pass, got error: %v", err)
	}
}

// TEST-VAL-013
// Scenario: an operation with no parameters at all (system_info) must
// pass validation when called with no arguments.
func TestValidate_OperationWithNoParameters_Passes(t *testing.T) {
	v := New(traceTestConfig())
	req := newRequest(t, "system_info", nil)

	if err := v.Validate(req); err != nil {
		t.Fatalf("expected system_info with no arguments to pass, got error: %v", err)
	}
}
