package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safeterm/internal/config"
)

// TEST-ISOLATION-002, TEST-ISOLATION-003, TEST-ISOLATION-005, TEST-ISOLATION-006, TEST-AUDIT-007
func TestCapabilityProfilesHaveSeparateHardenedSystemdUnits(t *testing.T) {
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 4 {
		t.Fatalf("profiles=%d, want 4", len(cfg.Profiles))
	}
	for _, profile := range cfg.Profiles {
		path := filepath.Join("..", "..", "deploy", "systemd", "safeterm-runner-"+string(profile.Name)+".service")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		unit := strings.ReplaceAll(string(data), "\r\n", "\n")
		requiredSettings := []string{
			"User=" + profile.RunAsUser,
			"ExecStart=/usr/local/bin/safeterm-runner --config /etc/safeterm --profile " + string(profile.Name),
			"ProtectSystem=strict", "ProtectHome=yes",
			"MemoryMax=", "TasksMax=", "RestrictAddressFamilies=AF_UNIX",
			"Requires=safeterm-audit.service", "SupplementaryGroups=safeterm-audit",
		}
		if profile.Name == config.ProfileServiceManager {
			requiredSettings = append(requiredSettings, "NoNewPrivileges=no", "RestrictSUIDSGID=no")
		} else {
			requiredSettings = append(requiredSettings, "NoNewPrivileges=yes", "RestrictSUIDSGID=yes")
		}
		for _, required := range requiredSettings {
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
	gateway := strings.ReplaceAll(string(gatewayData), "\r\n", "\n")
	if strings.Contains(gateway, "safeterm-runner --profile") || !strings.Contains(gateway, "CapabilityBoundingSet=\nAmbientCapabilities=\n") || !strings.Contains(gateway, "RestrictAddressFamilies=AF_UNIX") {
		t.Fatal("Gateway unit is executable-capable or insufficiently restricted")
	}
	if !strings.Contains(gateway, "Requires=safeterm-audit.service") || !strings.Contains(gateway, "SupplementaryGroups=safeterm-audit") {
		t.Fatal("Gateway is not bound to Audit Service")
	}
	auditData, err := os.ReadFile(filepath.Join("..", "..", "deploy", "systemd", "safeterm-audit.service"))
	if err != nil {
		t.Fatal(err)
	}
	auditUnit := strings.ReplaceAll(string(auditData), "\r\n", "\n")
	for _, required := range []string{
		"User=safeterm-audit", "StateDirectory=safeterm", "StateDirectoryMode=0700",
		"ExecStart=/usr/local/bin/safeterm-audit --store /var/lib/safeterm/audit.jsonl",
		"CapabilityBoundingSet=\nAmbientCapabilities=\n", "ProtectSystem=strict", "RestrictAddressFamilies=AF_UNIX",
	} {
		if !strings.Contains(auditUnit, required) {
			t.Errorf("audit unit missing %q", required)
		}
	}
}
