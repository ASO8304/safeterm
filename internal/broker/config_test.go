package broker

import (
	"bytes"
	"context"
	"testing"

	"safeterm/internal/configmanager"
)

type fakeConfigService struct {
	id    string
	actor configmanager.Actor
}

func (f *fakeConfigService) List(_ context.Context, a configmanager.Actor) ([]configmanager.ResourceSummary, error) {
	f.actor = a
	return []configmanager.ResourceSummary{{ID: f.id, Profile: "application", Format: configmanager.FormatINI, Access: configmanager.AccessReadWrite}}, nil
}
func (f *fakeConfigService) Describe(context.Context, configmanager.Actor, string) (configmanager.ResourceDescription, error) {
	return configmanager.ResourceDescription{}, nil
}
func (f *fakeConfigService) Read(context.Context, configmanager.Actor, string) (configmanager.ReadResult, error) {
	return configmanager.ReadResult{}, nil
}
func (f *fakeConfigService) ValidatePatch(context.Context, configmanager.Actor, configmanager.Patch) (configmanager.ValidationResult, error) {
	return configmanager.ValidationResult{Valid: true}, nil
}
func (f *fakeConfigService) Preview(context.Context, configmanager.Actor, configmanager.Patch) (configmanager.PreviewResult, error) {
	return configmanager.PreviewResult{}, nil
}
func (f *fakeConfigService) Apply(context.Context, configmanager.Actor, string) (configmanager.ApplyResult, error) {
	return configmanager.ApplyResult{}, nil
}
func (f *fakeConfigService) Rollback(context.Context, configmanager.Actor, string) (configmanager.RollbackResult, error) {
	return configmanager.RollbackResult{}, nil
}

func TestConfigBrokerRoutesAuthenticatedIdentityIntoRestrictedShell(t *testing.T) {
	service := &fakeConfigService{id: "cats-options"}
	configs, err := NewConfigBroker([]configmanager.Service{service})
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := NewCatalog(brokerTestConfig())
	output := &bytes.Buffer{}
	shell, err := NewShellWithConfig(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"netops"}}, bytes.NewReader(nil), output, configs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shell.executeLine(context.Background(), "config list", make(chan byte)); err != nil {
		t.Fatal(err)
	}
	if service.actor.BrokerType != "ssh" || service.actor.UserID != "alice" || len(service.actor.Roles) != 1 || service.actor.Roles[0] != "netops" {
		t.Fatalf("actor=%+v", service.actor)
	}
	if !bytes.Contains(output.Bytes(), []byte("cats-options")) {
		t.Fatalf("output=%q", output.String())
	}
}
