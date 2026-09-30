package translation

import (
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/validation"
)

var _ agent.Translator = (*Translator)(nil)

func tokenConfig(tokens ...string) *config.Config {
	return &config.Config{
		Version:     "test-v1",
		Profiles:    []config.ProfileDefinition{{Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/safeterm/test.sock", RunAsUser: "test", MemoryLimitMiB: 64, ProcessLimit: 8}},
		Executables: []config.ExecutableDefinition{{ID: "example", Path: "/usr/bin/example", SHA256: "0000000000000000000000000000000000000000000000000000000000000000", AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics}}},
		Operations: []config.OperationDefinition{{
			Name: "example", RunnerProfile: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, ExecutableRef: "example",
			ArgsTemplate: tokens,
			Parameters: []config.ParameterDefinition{
				{Name: "value", Type: config.ParamTypeString},
			},
		}}}
}

func mustTranslator(t *testing.T, cfg *config.Config) *Translator {
	t.Helper()
	tr, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func shippedConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func assertTranslation(t *testing.T, tr *Translator, req *contract.ExecutionRequest, command string, args []string) {
	t.Helper()
	gotCommand, gotArgs, err := tr.Translate(req)
	if err != nil || gotCommand != command || !reflect.DeepEqual(gotArgs, args) {
		t.Fatalf("Translate() = (%q, %#v, %v), want (%q, %#v, nil)", gotCommand, gotArgs, err, command, args)
	}
}

func assertRejected(t *testing.T, tr *Translator, req *contract.ExecutionRequest, message string) {
	t.Helper()
	command, args, err := tr.Translate(req)
	if err == nil || !strings.Contains(err.Error(), message) || command != "" || args != nil {
		t.Fatalf("expected empty output and error containing %q, got (%q, %#v, %v)", message, command, args, err)
	}
}

// TEST-TRANS-001
// Requirement: REQ-006 (AC-1)
// Preserve the exact trace example using the actual shipped configuration.
func TestTranslate_ExactTraceExample(t *testing.T) {
	cfg := shippedConfig(t)
	req := &contract.ExecutionRequest{Command: "trace", Arguments: map[string]string{"port": "999", "host": "192.168.10.20"}}
	if err := validation.New(cfg).Validate(req); err != nil {
		t.Fatal(err)
	}
	assertTranslation(t, mustTranslator(t, cfg), req, "/usr/bin/tcpdump", []string{"-i", "any", "port", "999", "and", "host", "192.168.10.20"})
}

// TEST-TRANS-002
// Requirement: REQ-006 (AC-2)
// A complete value remains one argument, including whitespace or an empty value.
func TestTranslate_PreservesCommandAndArgumentBoundaries(t *testing.T) {
	cfg := tokenConfig("literal with spaces", "{value}", "", "{value}", "--")
	cfg.Executables[0].Path = "/opt/tools/example tool"
	tr := mustTranslator(t, cfg)
	for _, value := range []string{"two words", "", "a\tb", "--flag other"} {
		t.Run(value, func(t *testing.T) {
			req := &contract.ExecutionRequest{Command: "example", Arguments: map[string]string{"value": value}}
			if err := validation.New(cfg).Validate(req); err != nil {
				t.Fatal(err)
			}
			assertTranslation(t, tr, req, "/opt/tools/example tool", []string{"literal with spaces", value, "", value, "--"})
		})
	}
}

// TEST-TRANS-003
// Requirement: REQ-006 (AC-3)
// Both empty template forms are supported; shipped system_info retains -a.
func TestTranslate_EmptyTemplatesAndSystemInfo(t *testing.T) {
	for _, tokens := range [][]string{nil, {}} {
		cfg := tokenConfig(tokens...)
		assertTranslation(t, mustTranslator(t, cfg), &contract.ExecutionRequest{Command: "example"}, "/usr/bin/example", []string{})
	}
	assertTranslation(t, mustTranslator(t, shippedConfig(t)), &contract.ExecutionRequest{Command: "system_info"}, "/usr/bin/uname", []string{"-a"})
}

// TEST-TRANS-004
// Requirement: REQ-006 (AC-4)
// Invalid configuration never yields a partially usable Translator.
func TestNew_RejectsInvalidConfig(t *testing.T) {
	for _, name := range []string{"nil", "empty executable", "duplicate parameter"} {
		t.Run(name, func(t *testing.T) {
			cfg := tokenConfig("{value}")
			want := ""
			switch name {
			case "nil":
				cfg, want = nil, "nil configuration"
			case "empty executable":
				cfg.Operations[0].ExecutableRef = ""
				want = "requires executable_ref"
			case "duplicate parameter":
				cfg.Operations[0].Parameters = append(cfg.Operations[0].Parameters, cfg.Operations[0].Parameters[0])
				want = "duplicate parameter"
			}
			tr, err := New(cfg)
			if tr != nil || err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("New() = (%v, %v), want nil and error containing %q", tr, err, want)
			}
		})
	}
}

// TEST-TRANS-005
// Requirement: REQ-006 (AC-5)
// No ambiguous or undeclared template syntax is accepted.
func TestNew_RejectsMalformedAndUndefinedPlaceholders(t *testing.T) {
	for _, token := range []string{"{", "}", "{}", "{value", "value}", "{{value}}", "{value}{value}", "--host={value}", "{value}/suffix", "{unknown}"} {
		t.Run(token, func(t *testing.T) {
			cfg := tokenConfig("literal", token)
			tr, err := New(cfg)
			if tr != nil || err == nil || !strings.Contains(err.Error(), `operation "example"`) || !strings.Contains(err.Error(), "args_template[1]") {
				t.Fatalf("New() = (%v, %v), want nil and template location error", tr, err)
			}
		})
	}
}

// TEST-TRANS-006
// Requirement: REQ-006 (AC-6)
// Missing substitutions fail even if validation treats that parameter as optional.
func TestTranslate_MissingSubstitutionReturnsNoPartialOutput(t *testing.T) {
	for _, required := range []bool{false, true} {
		cfg := tokenConfig("literal", "{value}")
		cfg.Operations[0].Parameters[0].Required = required
		for _, args := range []map[string]string{nil, {}} {
			assertRejected(t, mustTranslator(t, cfg), &contract.ExecutionRequest{Command: "example", Arguments: args}, `missing argument "value"`)
		}
	}
}

// TEST-TRANS-007
// Requirement: REQ-006 (AC-7)
// The request command can select only a configured logical operation.
func TestTranslate_RejectsNilOrUnknownOperation(t *testing.T) {
	tr := mustTranslator(t, shippedConfig(t))
	assertRejected(t, tr, nil, "nil request")
	for _, name := range []string{"", "missing", "tcpdump", "/usr/bin/uname", "system_info; uname"} {
		assertRejected(t, tr, &contract.ExecutionRequest{Command: name}, "unknown operation")
	}
	assertRejected(t, &Translator{}, &contract.ExecutionRequest{Command: "trace"}, "unknown operation")
	emptyConfig := tokenConfig()
	emptyConfig.Operations = nil
	assertRejected(t, mustTranslator(t, emptyConfig), &contract.ExecutionRequest{Command: "trace"}, "unknown operation")
}

// TEST-TRANS-008
// Requirement: REQ-006 (AC-8)
// Caller mutations cannot modify compiled templates or other returned slices.
func TestTranslate_SnapshotAndOutputsAreIndependent(t *testing.T) {
	cfg := tokenConfig("literal", "{value}")
	tr := mustTranslator(t, cfg)
	if !reflect.DeepEqual(cfg, tokenConfig("literal", "{value}")) {
		t.Fatal("New mutated the source config")
	}
	cfg.Executables[0].Path = "/changed"
	cfg.Operations[0].ArgsTemplate[0] = "changed"
	cfg.Operations[0].Parameters[0].Name = "changed"
	req := &contract.ExecutionRequest{Command: "example", Arguments: map[string]string{"value": "original"}}
	assertTranslation(t, tr, req, "/usr/bin/example", []string{"literal", "original"})
	_, args, err := tr.Translate(req)
	if err != nil {
		t.Fatal(err)
	}
	args[0] = "changed output"
	assertTranslation(t, tr, req, "/usr/bin/example", []string{"literal", "original"})
	if !reflect.DeepEqual(req, &contract.ExecutionRequest{Command: "example", Arguments: map[string]string{"value": "original"}}) {
		t.Fatal("Translate mutated the request")
	}
	req.Arguments["value"] = "second request"
	assertTranslation(t, tr, req, "/usr/bin/example", []string{"literal", "second request"})
	if args[1] != "original" {
		t.Fatal("request mutation changed previously returned arguments")
	}
}

// TEST-TRANS-009
// Requirement: REQ-006 (AC-9)
// Only configured tokens enter the argument list, not every request argument.
func TestTranslate_UnusedParameterDoesNotBecomeAnArgument(t *testing.T) {
	cfg := tokenConfig("literal")
	tr := mustTranslator(t, cfg)
	for _, args := range []map[string]string{nil, {"value": "unused"}} {
		req := &contract.ExecutionRequest{Command: "example", Arguments: args}
		if err := validation.New(cfg).Validate(req); err != nil {
			t.Fatal(err)
		}
		assertTranslation(t, tr, req, "/usr/bin/example", []string{"literal"})
	}
}
