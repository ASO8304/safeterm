package execution

import (
	"context"
	"testing"

	"safeterm/internal/agent"
	"safeterm/internal/config"
)

var _ agent.Executor = (*SystemdExecutor)(nil)

type fakeServiceController struct {
	calls        int
	action, unit string
}

func (c *fakeServiceController) Control(_ context.Context, action, unit string) (string, string, int, error) {
	c.calls++
	c.action, c.unit = action, unit
	return "controlled", "", 0, nil
}

func systemdConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Operations = append(cfg.Operations, config.OperationDefinition{Name: "restart_demo", RunnerProfile: config.ProfileServiceManager, ExecutorKind: config.ExecutorSystemd, Systemd: &config.SystemdOperationDefinition{Unit: "demo.service", Action: "restart"}})
	cfg.Policies = append(cfg.Policies, config.PolicyDefinition{Role: "netops", Operations: []string{"restart_demo"}})
	if err := config.ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TEST-ISOLATION-007
func TestSystemdExecutorAllowsOnlyCompiledUnitAction(t *testing.T) {
	controller := &fakeServiceController{}
	executor, err := NewSystemdExecutor(systemdConfig(t), controller)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exitCode, err := executor.Execute(context.Background(), "demo.service", []string{"restart"})
	if err != nil || stdout != "controlled" || stderr != "" || exitCode != 0 || controller.calls != 1 || controller.action != "restart" || controller.unit != "demo.service" {
		t.Fatalf("unexpected result: %q %q %d %v controller=%+v", stdout, stderr, exitCode, err, controller)
	}
}

// TEST-ISOLATION-007, TEST-ISOLATION-011
func TestSystemdExecutorRejectsFreeFormInputsBeforeController(t *testing.T) {
	controller := &fakeServiceController{}
	executor, _ := NewSystemdExecutor(systemdConfig(t), controller)
	for _, tc := range []struct {
		unit string
		args []string
	}{
		{"other.service", []string{"restart"}},
		{"demo.service", []string{"start"}},
		{"demo.service", []string{"restart", "--no-block"}},
		{"demo.service;id", []string{"restart"}},
	} {
		if _, _, code, err := executor.Execute(context.Background(), tc.unit, tc.args); err == nil || code != ExitCodeUnavailable {
			t.Fatalf("accepted %#v", tc)
		}
	}
	if controller.calls != 0 {
		t.Fatalf("controller called %d times", controller.calls)
	}
}
