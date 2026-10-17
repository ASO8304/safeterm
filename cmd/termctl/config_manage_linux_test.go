//go:build linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configFixture(t *testing.T) (configRuntime, *catalogFakeHost, []byte, []byte) {
	t.Helper()
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	registry := []byte(`{"version":"test","resources":[{"id":"options","description":"App","profile":"application","path":"/etc/app/options.ini","format":"ini","access":"read-only","fields":[{"section":"Server","key":"Port","type":"integer"}],"permissions":{"read_roles":["viewer"]}}]}`)
	oldPlan := []byte("[Service]\nReadWritePaths=/old/app\n")
	for path, data := range map[string][]byte{filepath.Join(dir, "resources.json"): registry, filepath.Join(state, "verified.json"): registry, filepath.Join(dir, "writer.conf"): oldPlan} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	host := &catalogFakeHost{active: map[string]bool{serviceUnits["config-writer"]: true, serviceUnits["gateway"]: true, serviceUnits["config-application"]: true, serviceUnits["ssh"]: true}}
	return configRuntime{filepath.Join(dir, "resources.json"), filepath.Join(dir, "writer.conf"), state, host.run}, host, registry, oldPlan
}

func TestConfigurationApplyPreservesInactiveServicesAndPublishesCheckpoint(t *testing.T) {
	r, host, oldRegistry, _ := configFixture(t)
	changed := bytes.Replace(oldRegistry, []byte("\"test\""), []byte("\"updated\""), 1)
	if err := os.WriteFile(r.registry, changed, 0600); err != nil {
		t.Fatal(err)
	}
	plan := []byte("[Service]\nReadWritePaths=/new/app\n")
	if err := r.apply(plan); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(r.dropin)
	if !bytes.Equal(data, plan) {
		t.Fatal("plan not applied")
	}
	data, _ = os.ReadFile(filepath.Join(r.state, "verified.json"))
	if !bytes.Equal(data, changed) {
		t.Fatal("checkpoint not updated")
	}
	if host.active[serviceUnits["config-database"]] || host.active[serviceUnits["config-network"]] {
		t.Fatal("inactive managers were started")
	}
	if _, err := os.Stat(filepath.Join(r.state, "pending")); !os.IsNotExist(err) {
		t.Fatal("journal not cleaned")
	}
}

func TestConfigurationApplyRestoresRegistryAndSandboxAfterFailure(t *testing.T) {
	r, host, oldRegistry, oldPlan := configFixture(t)
	if err := os.WriteFile(r.registry, bytes.Replace(oldRegistry, []byte("\"test\""), []byte("\"updated\""), 1), 0600); err != nil {
		t.Fatal(err)
	}
	failed := false
	host.fail = func(name string, args []string) bool {
		if !failed && strings.HasSuffix(name, "systemctl") && len(args) > 1 && args[0] == "start" && args[1] == serviceUnits["config-application"] {
			failed = true
			return true
		}
		return false
	}
	if err := r.apply([]byte("new plan")); err == nil {
		t.Fatal("injected start failure ignored")
	}
	data, _ := os.ReadFile(r.registry)
	if !bytes.Equal(data, oldRegistry) {
		t.Fatal("previous verified registry not restored")
	}
	data, _ = os.ReadFile(r.dropin)
	if !bytes.Equal(data, oldPlan) {
		t.Fatal("sandbox not restored")
	}
	for _, name := range []string{"config-writer", "gateway", "config-application", "ssh"} {
		if !host.active[serviceUnits[name]] {
			t.Fatalf("service not recovered: %s", name)
		}
	}
}

func TestConfigurationRecoveryFailureRetainsJournal(t *testing.T) {
	r, host, _, _ := configFixture(t)
	host.fail = func(name string, args []string) bool { return len(args) > 0 && args[0] == "stop" }
	if err := r.apply([]byte("new plan")); err == nil || !strings.Contains(err.Error(), "config rollback") {
		t.Fatalf("missing recovery guidance: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.state, "pending")); err != nil {
		t.Fatal("lost recovery journal")
	}
	host.fail = nil
	if err := r.recover(); err != nil {
		t.Fatal(err)
	}
}
