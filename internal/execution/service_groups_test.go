package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"safeterm/internal/config"
)

type stepController struct {
	steps   []config.ServiceStep
	control func(context.Context, int) (int, error)
}

func (c *stepController) Control(ctx context.Context, action, unit string) (string, string, int, error) {
	c.steps = append(c.steps, config.ServiceStep{Unit: unit, Action: action})
	code := 0
	var err error
	if c.control != nil {
		code, err = c.control(ctx, len(c.steps))
	}
	return "secret-real.service stdout", "secret-real.service stderr", code, err
}
func groupConfig(t *testing.T, action string) *config.Config {
	cfg := systemdConfig(t)
	cfg.Operations = append(cfg.Operations, config.OperationDefinition{Name: "group", RunnerProfile: config.ProfileServiceManager, ExecutorKind: config.ExecutorSystemd,
		Systemd: &config.SystemdOperationDefinition{Units: []string{"one.service", "two.service", "three.service"}, Alias: "akbar-all", Action: action}})
	return cfg
}

func TestServiceGroupOrdersAndSanitizesOutput(t *testing.T) {
	for _, tc := range []struct {
		action string
		want   []config.ServiceStep
	}{
		{"start", []config.ServiceStep{{Unit: "one.service", Action: "start"}, {Unit: "two.service", Action: "start"}, {Unit: "three.service", Action: "start"}}},
		{"stop", []config.ServiceStep{{Unit: "three.service", Action: "stop"}, {Unit: "two.service", Action: "stop"}, {Unit: "one.service", Action: "stop"}}},
		{"restart", []config.ServiceStep{{Unit: "three.service", Action: "stop"}, {Unit: "two.service", Action: "stop"}, {Unit: "one.service", Action: "stop"}, {Unit: "one.service", Action: "start"}, {Unit: "two.service", Action: "start"}, {Unit: "three.service", Action: "start"}}},
		{"disable", []config.ServiceStep{{Unit: "three.service", Action: "disable"}, {Unit: "two.service", Action: "disable"}, {Unit: "one.service", Action: "disable"}}},
		{"enable", []config.ServiceStep{{Unit: "one.service", Action: "enable"}, {Unit: "two.service", Action: "enable"}, {Unit: "three.service", Action: "enable"}}},
		{"status", []config.ServiceStep{{Unit: "one.service", Action: "status"}, {Unit: "two.service", Action: "status"}, {Unit: "three.service", Action: "status"}}},
	} {
		t.Run(tc.action, func(t *testing.T) {
			c := &stepController{}
			cfg := groupConfig(t, tc.action)
			e, err := NewSystemdExecutor(cfg, c)
			if err != nil {
				t.Fatal(err)
			}
			// Changing the caller's catalog must not change the compiled plan.
			cfg.Operations[len(cfg.Operations)-1].Systemd.Units[0] = "tampered.service"
			out, stderr, code, err := e.Execute(context.Background(), "group", []string{tc.action})
			if err != nil || code != 0 || stderr != "" || !reflect.DeepEqual(c.steps, tc.want) {
				t.Fatalf("steps=%v output=%s err=%v", c.steps, out, err)
			}
			if !strings.Contains(out, "akbar-all") || strings.Contains(out, ".service") || strings.Contains(out, "secret") {
				t.Fatalf("unsafe output %q", out)
			}
		})
	}
}

func TestGroupStopsAtEveryFailedStepWithoutRecovery(t *testing.T) {
	for failed := 1; failed <= 6; failed++ {
		c := &stepController{control: func(_ context.Context, index int) (int, error) {
			if index == failed {
				return 1, errors.New("secret-real.service failed")
			}
			return 0, nil
		}}
		e, err := NewSystemdExecutor(groupConfig(t, "restart"), c)
		if err != nil {
			t.Fatal(err)
		}
		out, stderr, code, err := e.Execute(context.Background(), "group", []string{"restart"})
		if err == nil || code == 0 || len(c.steps) != failed {
			t.Fatalf("step=%d calls=%v err=%v", failed, c.steps, err)
		}
		if strings.Contains(out+stderr+err.Error(), ".service") {
			t.Fatal("leaked unit name")
		}
		// Failure must release the reservation for an explicit retry.
		c.control = nil
		c.steps = nil
		if _, _, _, err := e.Execute(context.Background(), "group", []string{"restart"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestServiceStepAuditGatesExecutionAndFollowingSteps(t *testing.T) {
	for _, failOutcome := range []string{"service-step-started", "service-step-completed"} {
		c := &stepController{}
		e, _ := NewSystemdExecutor(groupConfig(t, "restart"), c)
		events := []ServiceStepEvent{}
		ctx := WithServiceReporter(context.Background(), func(_ context.Context, event ServiceStepEvent) error {
			events = append(events, event)
			if event.Outcome == failOutcome {
				return errors.New("unavailable")
			}
			return nil
		})
		if _, _, _, err := e.Execute(ctx, "group", []string{"restart"}); err == nil {
			t.Fatal("audit failure ignored")
		}
		want := 0
		if failOutcome == "service-step-completed" {
			want = 1
		}
		if len(c.steps) != want {
			t.Fatalf("calls=%v", c.steps)
		}
		if len(events) == 0 || events[0].Unit != "three.service" || events[0].Index != 1 {
			t.Fatalf("audit=%v", events)
		}
	}
}

func TestServiceGroupTimeoutAndCancellationDoNotContinue(t *testing.T) {
	c := &stepController{control: func(ctx context.Context, _ int) (int, error) { <-ctx.Done(); return -1, ctx.Err() }}
	cfg := groupConfig(t, "restart")
	cfg.Operations[len(cfg.Operations)-1].Systemd.TimeoutSeconds = 1
	e, _ := NewSystemdExecutor(cfg, c)
	before := time.Now()
	_, _, _, err := e.Execute(context.Background(), "group", []string{"restart"})
	if err == nil || len(c.steps) != 1 || time.Since(before) > 3*time.Second {
		t.Fatalf("unbounded sequence: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.steps = nil
	if _, _, _, err := e.Execute(ctx, "group", []string{"restart"}); err == nil || len(c.steps) != 0 {
		t.Fatal("canceled request executed")
	}
}

func TestServiceGroupRejectsOverlapButAllowsUnrelatedUnits(t *testing.T) {
	entered := make(chan struct{})
	unblock := make(chan struct{})
	c := &stepController{control: func(_ context.Context, index int) (int, error) {
		if index == 1 {
			close(entered)
			<-unblock
		}
		return 0, nil
	}}
	cfg := groupConfig(t, "restart")
	cfg.Operations = append(cfg.Operations, config.OperationDefinition{Name: "single", RunnerProfile: config.ProfileServiceManager, ExecutorKind: config.ExecutorSystemd, Systemd: &config.SystemdOperationDefinition{Unit: "one.service", Action: "stop"}})
	e, _ := NewSystemdExecutor(cfg, c)
	done := make(chan error, 1)
	go func() { _, _, _, err := e.Execute(context.Background(), "group", []string{"restart"}); done <- err }()
	<-entered
	if _, _, _, err := e.Execute(context.Background(), "single", []string{"stop"}); err == nil {
		close(unblock)
		t.Fatal("overlapping operation allowed")
	}
	close(unblock)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var locks ServiceLocks
	release, ok := locks.Acquire([]string{"one"})
	if !ok {
		t.Fatal("lock failed")
	}
	other, ok := locks.Acquire([]string{"two"})
	if !ok {
		t.Fatal("unrelated units blocked")
	}
	other()
	release()
	release()
}

func TestServiceStatusDoesNotLeakInactiveOrErrorDetails(t *testing.T) {
	c := &stepController{control: func(context.Context, int) (int, error) { return 3, errors.New("secret-real.service inactive") }}
	e, _ := NewSystemdExecutor(groupConfig(t, "status"), c)
	out, stderr, code, err := e.Execute(context.Background(), "group", []string{"status"})
	if err != nil || code != 0 || stderr != "" || !strings.Contains(out, "inactive or failed") || strings.Contains(out, ".service") {
		t.Fatalf("%s %s %d %v", out, stderr, code, err)
	}
}
