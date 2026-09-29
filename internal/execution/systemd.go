package execution

import (
	"context"
	"fmt"
	"path/filepath"

	"safeterm/internal/config"
)

// ServiceController is the narrow transport used by the typed systemd backend.
type ServiceController interface {
	Control(ctx context.Context, action, unit string) (stdout, stderr string, exitCode int, err error)
}

// SystemdExecutor implements agent.Executor but accepts only unit/action pairs
// compiled from the active service-manager catalog.
type SystemdExecutor struct {
	allowed    map[string]map[string]bool
	controller ServiceController
}

func NewSystemdExecutor(cfg *config.Config, controller ServiceController) (*SystemdExecutor, error) {
	if controller == nil {
		return nil, fmt.Errorf("execution: systemd controller is required")
	}
	if err := config.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("execution: invalid systemd catalog: %w", err)
	}
	allowed := map[string]map[string]bool{}
	for _, operation := range cfg.Operations {
		if operation.ExecutorKind != config.ExecutorSystemd {
			continue
		}
		if allowed[operation.Systemd.Unit] == nil {
			allowed[operation.Systemd.Unit] = map[string]bool{}
		}
		allowed[operation.Systemd.Unit][operation.Systemd.Action] = true
	}
	return &SystemdExecutor{allowed: allowed, controller: controller}, nil
}

func (e *SystemdExecutor) Execute(ctx context.Context, unit string, args []string) (string, string, int, error) {
	if e == nil || e.controller == nil || ctx == nil {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: invalid systemd executor call")
	}
	if len(args) != 1 {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: systemd request must contain exactly one action")
	}
	action := args[0]
	if e.allowed[unit] == nil || !e.allowed[unit][action] {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: systemd unit/action is not allowlisted")
	}
	return e.controller.Control(ctx, action, unit)
}

// SystemctlController is a Linux adapter for the typed backend. The executable
// and argv shape are fixed here; untrusted values never select flags or a shell.
type SystemctlController struct {
	runner *Runner
	path   string
}

func NewSystemctlController(path string, timeoutRunner *Runner) (*SystemctlController, error) {
	if !filepath.IsAbs(path) || timeoutRunner == nil {
		return nil, fmt.Errorf("execution: systemctl path and runner are required")
	}
	return &SystemctlController{runner: timeoutRunner, path: path}, nil
}

func (c *SystemctlController) Control(ctx context.Context, action, unit string) (string, string, int, error) {
	return c.runner.Execute(ctx, c.path, []string{action, unit})
}
