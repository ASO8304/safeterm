package execution

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"safeterm/internal/agent"
	"safeterm/internal/config"
)

// ServiceController is the narrow transport used by the typed systemd backend.
type ServiceController interface {
	Control(ctx context.Context, action, unit string) (stdout, stderr string, exitCode int, err error)
}

// ServiceLocks atomically reserves a unit set. Refusal rather than queuing keeps
// lock ordering independent of administrator-defined startup order.
// Gateway shares one instance across profiles; each Runner also guards locally.
type ServiceLocks struct {
	mu   sync.Mutex
	busy map[string]bool
}

func (l *ServiceLocks) Acquire(units []string) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.busy == nil {
		l.busy = map[string]bool{}
	}
	for _, unit := range units {
		if l.busy[unit] {
			return nil, false
		}
	}
	for _, unit := range units {
		l.busy[unit] = true
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			for _, unit := range units {
				delete(l.busy, unit)
			}
		})
	}, true
}

type ServiceStepEvent struct {
	Unit    string
	Action  string
	Index   int
	Outcome string
}
type serviceReporterKey struct{}
type ServiceReporter func(context.Context, ServiceStepEvent) error

func WithServiceReporter(ctx context.Context, reporter ServiceReporter) context.Context {
	return context.WithValue(ctx, serviceReporterKey{}, reporter)
}

// SystemdExecutor resolves signed operation IDs to immutable unit/action plans.
// Public output never includes controller stdout/stderr or physical unit names.
type SystemdExecutor struct {
	plans      map[string]config.SystemdOperationDefinition
	controller ServiceController
	locks      ServiceLocks
}

func NewSystemdExecutor(cfg *config.Config, controller ServiceController) (*SystemdExecutor, error) {
	if controller == nil {
		return nil, fmt.Errorf("execution: systemd controller is required")
	}
	if err := config.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("execution: invalid systemd catalog: %w", err)
	}
	plans := map[string]config.SystemdOperationDefinition{}
	for _, op := range cfg.Operations {
		if op.ExecutorKind != config.ExecutorSystemd {
			continue
		}
		plan := *op.Systemd
		plan.Units = append([]string(nil), plan.Units...)
		if plan.Alias == "" {
			plan.Alias = op.Name
		}
		plans[op.Name] = plan
	}
	return &SystemdExecutor{plans: plans, controller: controller}, nil
}
func (e *SystemdExecutor) Execute(ctx context.Context, operation string, args []string) (string, string, int, error) {
	if e == nil || e.controller == nil || ctx == nil {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: invalid systemd executor call")
	}
	plan, ok := e.plans[operation]
	if !ok || len(args) != 1 || args[0] != plan.Action {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: service operation/action is not allowlisted")
	}
	release, ok := e.locks.Acquire(plan.OrderedUnits())
	if !ok {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: service operation conflicts with another active operation")
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(plan.Timeout())*time.Second)
	defer cancel()
	reporter, _ := ctx.Value(serviceReporterKey{}).(ServiceReporter)
	var output strings.Builder
	report := func(step config.ServiceStep, index int, outcome string) error {
		if reporter == nil {
			return nil
		}
		reportCtx := ctx
		cancel := func() {}
		if outcome != "service-step-started" {
			reportCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		}
		defer cancel()
		return reporter(reportCtx, ServiceStepEvent{Unit: step.Unit, Action: step.Action, Index: index, Outcome: outcome})
	}
	for i, step := range plan.Steps() {
		fail := func(reason string) (string, string, int, error) {
			return output.String(), "", ExitCodeUnavailable, fmt.Errorf("execution: %s: step %d (%s) %s; sequence stopped; no automatic recovery", plan.Alias, i+1, step.Action, reason)
		}
		if ctx.Err() != nil {
			return fail("canceled or timed out")
		}
		if err := report(step, i+1, "service-step-started"); err != nil {
			return fail("audit unavailable before execution")
		}
		if ctx.Err() != nil {
			if err := report(step, i+1, "service-step-failed"); err != nil {
				return fail("canceled and audit unavailable")
			}
			return fail("canceled before execution")
		}
		_, _, code, err := e.controller.Control(ctx, step.Action, step.Unit)
		// systemctl status uses 3 for an inactive/failed unit; render that state
		// without forwarding raw systemd/journal output. Status is a read operation;
		// reporting an inactive state is a successful query, not a successful start.
		inactive := step.Action == "status" && code == 3
		if ctx.Err() != nil || (!inactive && (err != nil || code != 0)) {
			if auditErr := report(step, i+1, "service-step-failed"); auditErr != nil {
				return fail("failed and audit unavailable")
			}
			return fail("failed or timed out")
		}
		if err := report(step, i+1, "service-step-completed"); err != nil {
			return fail("finished but audit unavailable")
		}
		state := "ok"
		if step.Action == "status" {
			state = "active"
			if inactive {
				state = "inactive or failed"
			}
		}
		fmt.Fprintf(&output, "%s: step %d/%d %s: %s\n", plan.Alias, i+1, len(plan.Steps()), step.Action, state)
	}
	return output.String(), "", 0, nil
}

// SystemctlController is a Linux adapter for the typed backend. The executable
// and argv shape are fixed here; untrusted values never select flags or a shell.
type SystemctlController struct {
	runner agent.Executor
	path   string
	prefix []string
}

type executableVerifier interface {
	VerifyExecutable(command string) error
}

func NewSystemctlController(path string, timeoutRunner agent.Executor) (*SystemctlController, error) {
	return NewSystemctlControllerWithPrefix(path, nil, timeoutRunner)
}

func NewSystemctlControllerWithPrefix(path string, prefix []string, timeoutRunner agent.Executor) (*SystemctlController, error) {
	if !filepath.IsAbs(path) || timeoutRunner == nil {
		return nil, fmt.Errorf("execution: systemctl path and runner are required")
	}
	return &SystemctlController{runner: timeoutRunner, path: path, prefix: append([]string(nil), prefix...)}, nil
}

func (c *SystemctlController) Control(ctx context.Context, action, unit string) (string, string, int, error) {
	if verifier, ok := c.runner.(executableVerifier); ok {
		for _, argument := range c.prefix {
			if filepath.IsAbs(argument) {
				if err := verifier.VerifyExecutable(argument); err != nil {
					return "", "", ExitCodeUnavailable, fmt.Errorf("execution: verify controller dependency: %w", err)
				}
			}
		}
	}
	args := append(append([]string(nil), c.prefix...), action, unit)
	return c.runner.Execute(ctx, c.path, args)
}
