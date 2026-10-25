//go:build linux

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"safeterm/internal/agent"
	"safeterm/internal/audit"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/execution"
	"safeterm/internal/isolation"
	"safeterm/internal/policy"
	"safeterm/internal/resources"
	"safeterm/internal/translation"
	"safeterm/internal/validation"
)

type groupAuditSink struct {
	mu     sync.Mutex
	events []audit.Event
}

func (s *groupAuditSink) Record(_ context.Context, e audit.Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}

type groupController struct{ steps []config.ServiceStep }

func (c *groupController) Control(_ context.Context, action, unit string) (string, string, int, error) {
	c.steps = append(c.steps, config.ServiceStep{Unit: unit, Action: action})
	return "physical-unit.service", "", 0, nil
}

func TestGroupedServiceAcrossAuthenticatedRunnerIPCAndAudit(t *testing.T) {
	base, err := config.LoadConfig("../../internal/defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	item, err := resources.Parse("app.json", []byte(`{"id":"application","description":"App","service":{"units":["database.service","web.service"],"actions":{"restart":{"roles":["service-operator"]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, _, err := resources.Compile(base, []resources.Named{item})
	if err != nil {
		t.Fatal(err)
	}
	store, err := isolation.NewStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := store.Current().ProfileConfig(config.ProfileServiceManager)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := translation.New(profile)
	if err != nil {
		t.Fatal(err)
	}
	controller := &groupController{}
	executor, err := execution.NewSystemdExecutor(profile, controller)
	if err != nil {
		t.Fatal(err)
	}
	sink := &groupAuditSink{}
	handler, err := isolation.NewProfileHandlerWithSink(config.ProfileServiceManager, store, agent.NewCore(validation.New(profile), policy.New(profile), tr, executor), sink)
	if err != nil {
		t.Fatal(err)
	}
	uid := uint32(os.Getuid())
	socket := filepath.Join(t.TempDir(), "runner.sock")
	server, err := isolation.ListenUnix(socket, 0600, []uint32{uid}, handler)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	client, err := isolation.NewUnixClient(socket, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := isolation.NewGatewayWithSink(store, map[config.RunnerProfile]isolation.Client{config.ProfileServiceManager: client}, sink)
	if err != nil {
		t.Fatal(err)
	}
	operation := resources.OperationID("application", "service", "restart")
	request, err := contract.NewExecutionRequest(operation, nil, contract.Metadata{BrokerType: "ssh", UserID: "operator", ClaimedRoles: []string{"service-operator"}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := gateway.Dispatch(ctx, request)
	if err != nil || response.Result != contract.ResultSuccess || len(controller.steps) != 4 {
		t.Fatalf("response=%+v err=%v steps=%v", response, err, controller.steps)
	}
	if strings.Contains(response.Stdout+response.Stderr+response.Message, ".service") {
		t.Fatal("physical unit leaked through IPC")
	}
	started, completed := 0, 0
	for _, event := range sink.events {
		step, ok := audit.ParseServiceStep(event.Operation)
		if !ok {
			continue
		}
		if event.Outcome == "accepted" {
			started++
		}
		if event.Outcome == "completed" {
			completed++
		}
		if event.RequestID != request.RequestID || event.UserID != "operator" || step.ParentOperation != operation || step.Unit == "" {
			t.Fatalf("unattributed step %+v", event)
		}
	}
	if started != 4 || completed != 4 {
		t.Fatalf("missing audit steps %d/%d", started, completed)
	}
	request.Metadata.ClaimedRoles = []string{"viewer"}
	if _, err := gateway.Dispatch(ctx, request); err == nil {
		t.Fatal("viewer invoked service group")
	}
	if len(controller.steps) != 4 {
		t.Fatal("unauthorized request reached controller")
	}
}
