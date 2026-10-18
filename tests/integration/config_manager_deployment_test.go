package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safeterm/internal/configmanager"
)

func TestConfigManagerDeploymentProfilesAreIsolated(t *testing.T) {
	profiles := map[string]string{"application": "safeterm-config-application", "database": "safeterm-config-database", "network": "safeterm-config-network"}
	for profile, user := range profiles {
		path := filepath.Join("..", "..", "deploy", "systemd", "safeterm-config-manager-"+profile+".service")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		unit := string(data)
		for _, required := range []string{"User=" + user, "Group=safeterm-clients", "SupplementaryGroups=safeterm-audit", "NoNewPrivileges=yes", "ProtectSystem=strict", "RestrictAddressFamilies=AF_UNIX", "CapabilityBoundingSet=", "-profile=" + profile, "-run-as-user=" + user, "ReadWritePaths=/etc/safeterm-managed/" + profile} {
			if !strings.Contains(unit, required) {
				t.Errorf("%s missing %q", path, required)
			}
		}
		if strings.Contains(unit, "CAP_SYS_ADMIN") || strings.Contains(unit, "/bin/sh") {
			t.Errorf("%s grants unsafe capability", path)
		}
	}
}

func TestExampleResourceRegistryIsStrictAndProfileScoped(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "runtime", "resources.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry configmanager.Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	if err := configmanager.ValidateRegistry(&registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.Resources) != 0 {
		t.Fatalf("resources=%d", len(registry.Resources))
	}
	paths := map[string]bool{}
	for _, resource := range registry.Resources {
		if paths[resource.Path] {
			t.Fatal("shared managed path")
		}
		paths[resource.Path] = true
		if !strings.HasPrefix(resource.Path, "/etc/safeterm-managed/"+resource.Profile+"/") {
			t.Errorf("resource %s escapes profile directory", resource.ID)
		}
	}
}
