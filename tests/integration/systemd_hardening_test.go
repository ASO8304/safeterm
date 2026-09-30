package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safeterm/internal/config"
)

// TEST-ISOLATION-002, TEST-ISOLATION-003, TEST-ISOLATION-005, TEST-ISOLATION-006
func TestCapabilityProfilesHaveSeparateHardenedSystemdUnits(t *testing.T) {
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 3 {
		t.Fatalf("profiles=%d, want 3", len(cfg.Profiles))
	}
	for _, profile := range cfg.Profiles {
		path := filepath.Join("..", "..", "deploy", "systemd", "safeterm-runner-"+string(profile.Name)+".service")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		unit := string(data)
		for _, required := range []string{
			"User=" + profile.RunAsUser,
			"ExecStart=/usr/local/bin/safeterm-runner --config /etc/safeterm --profile " + string(profile.Name),
			"NoNewPrivileges=yes", "ProtectSystem=strict", "ProtectHome=yes",
			"MemoryMax=", "TasksMax=", "RestrictAddressFamilies=AF_UNIX",
		} {
			if !strings.Contains(unit, required) {
				t.Errorf("%s missing %q", path, required)
			}
		}
		if profile.Name == config.ProfileNetworkObserver {
			if !strings.Contains(unit, "CapabilityBoundingSet=CAP_NET_RAW") || !strings.Contains(unit, "AmbientCapabilities=CAP_NET_RAW") {
				t.Errorf("network unit lacks narrow capability")
			}
		} else if !strings.Contains(unit, "CapabilityBoundingSet=\nAmbientCapabilities=\n") {
			t.Errorf("%s does not drop all capabilities", path)
		}
	}
	gatewayData, err := os.ReadFile(filepath.Join("..", "..", "deploy", "systemd", "safeterm-gateway.service"))
	if err != nil {
		t.Fatal(err)
	}
	gateway := string(gatewayData)
	if strings.Contains(gateway, "safeterm-runner --profile") || !strings.Contains(gateway, "CapabilityBoundingSet=\nAmbientCapabilities=\n") || !strings.Contains(gateway, "RestrictAddressFamilies=AF_UNIX") {
		t.Fatal("Gateway unit is executable-capable or insufficiently restricted")
	}
}
