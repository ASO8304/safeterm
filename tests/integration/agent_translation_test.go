package integration

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/policy"
	"safeterm/internal/translation"
	"safeterm/internal/validation"
)

type observedTranslator struct {
	delegate agent.Translator
	calls    int
}

func (tr *observedTranslator) Translate(req *contract.ExecutionRequest) (string, []string, error) {
	tr.calls++
	return tr.delegate.Translate(req)
}

// captureExecutor records the exact argument boundaries without invoking the OS.
type captureExecutor struct {
	calls   int
	command string
	args    []string
}

func (ex *captureExecutor) Execute(_ context.Context, command string, args []string) (string, string, int, error) {
	ex.calls++
	ex.command = command
	ex.args = append([]string(nil), args...)
	return "captured", "", 0, nil
}

func translationConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func buildCoreWithRealTranslator(t *testing.T, cfg *config.Config) (*agent.Core, *observedTranslator, *captureExecutor) {
	t.Helper()
	tr, err := translation.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	observed := &observedTranslator{delegate: tr}
	ex := &captureExecutor{}
	return agent.NewCore(validation.New(cfg), policy.New(cfg), observed, ex), observed, ex
}

// TEST-INTEG-007
// Requirement: REQ-006 (AC-1, AC-3, AC-10), REQ-002 (AC-5, AC-6)
// The real M3-M6 stages deliver actual command/argv to a test-only Executor.
func TestCore_WithRealTranslator_CapturesConfiguredCommands(t *testing.T) {
	for _, tc := range []struct {
		operation, role, command string
		args                     []string
	}{
		{"trace", "netops", "/usr/bin/tcpdump", []string{"-i", "any", "port", "999", "and", "host", "192.168.10.20"}},
		{"system_info", "viewer", "/usr/bin/uname", []string{"-a"}},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			core, tr, ex := buildCoreWithRealTranslator(t, translationConfig(t))
			req := policyRequest(t, tc.operation, []string{tc.role})
			resp := core.HandleRequest(context.Background(), req)
			assertPolicyResponse(t, req, resp)
			if resp.Result != contract.ResultSuccess || resp.Stdout != "captured" || resp.ExitCode != 0 {
				t.Fatalf("expected successful captured execution: %+v", resp)
			}
			if tr.calls != 1 || ex.calls != 1 || ex.command != tc.command || !reflect.DeepEqual(ex.args, tc.args) {
				t.Fatalf("translation calls=%d, captured execution=%+v; want command=%q args=%q", tr.calls, ex, tc.command, tc.args)
			}
		})
	}
}

// TEST-INTEG-008
// Requirement: REQ-006 (AC-11), REQ-004 (AC-1, AC-2, AC-4, AC-6, AC-7), REQ-005 (AC-1, AC-2, AC-4)
// Invalid requests and unauthorized roles cannot reach the real Translator.
func TestCore_WithRealTranslator_EarlierStagesStopTranslation(t *testing.T) {
	for _, name := range []string{"unknown operation", "raw command", "missing argument", "port injection", "host injection", "extra argument", "viewer", "missing roles", "unknown role"} {
		t.Run(name, func(t *testing.T) {
			core, tr, ex := buildCoreWithRealTranslator(t, translationConfig(t))
			req := policyRequest(t, "trace", []string{"netops"})
			stage := "validation: "
			switch name {
			case "unknown operation":
				req.Command = "missing"
			case "raw command":
				req.Command = "tcpdump"
			case "missing argument":
				delete(req.Arguments, "host")
			case "port injection":
				req.Arguments["port"] = "; rm -rf /"
			case "host injection":
				req.Arguments["host"] = "192.168.10.20; whoami"
			case "extra argument":
				req.Arguments["command"] = "sh"
			case "viewer":
				req.Metadata.ClaimedRoles, stage = []string{"viewer"}, "policy: "
			case "missing roles":
				req.Metadata.ClaimedRoles, stage = nil, "policy: "
			case "unknown role":
				req.Metadata.ClaimedRoles, stage = []string{"ghost"}, "policy: "
			}
			resp := core.HandleRequest(context.Background(), req)
			assertPolicyResponse(t, req, resp)
			if resp.Result != contract.ResultRejected || !strings.HasPrefix(resp.Message, stage) {
				t.Fatalf("expected rejection from %s, got %+v", stage, resp)
			}
			if tr.calls != 0 || ex.calls != 0 {
				t.Fatalf("unexpected downstream calls: translation=%d execution=%d", tr.calls, ex.calls)
			}
		})
	}
}

// TEST-INTEG-009
// Requirement: REQ-006 (AC-6, AC-11), REQ-002 (AC-3)
// An optional parameter may pass validation yet be required by the template;
// a missing substitution must then fail at translation, before execution.
func TestCore_WithRealTranslator_MissingSubstitutionStopsExecution(t *testing.T) {
	cfg := translationConfig(t)
	for i := range cfg.Operations {
		if cfg.Operations[i].Name == "trace" {
			cfg.Operations[i].Parameters = append(cfg.Operations[i].Parameters, config.ParameterDefinition{Name: "label", Type: config.ParamTypeString})
			cfg.Operations[i].ArgsTemplate = append(cfg.Operations[i].ArgsTemplate, "{label}")
		}
	}
	core, tr, ex := buildCoreWithRealTranslator(t, cfg)
	req := policyRequest(t, "trace", []string{"netops"})
	resp := core.HandleRequest(context.Background(), req)
	assertPolicyResponse(t, req, resp)
	if resp.Result != contract.ResultRejected || !strings.HasPrefix(resp.Message, "translation: ") || !strings.Contains(resp.Message, `missing argument "label"`) {
		t.Fatalf("expected missing substitution rejection, got %+v", resp)
	}
	if tr.calls != 1 || ex.calls != 0 {
		t.Fatalf("unexpected stage calls: translation=%d execution=%d", tr.calls, ex.calls)
	}
}
