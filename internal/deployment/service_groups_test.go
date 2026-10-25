package deployment

import (
	"path/filepath"
	"strings"
	"testing"

	"safeterm/internal/config"
)

func TestGroupSudoersGrantsOnlyExactExpandedSteps(t *testing.T) {
	cfg, err := config.LoadConfig(filepath.Join("..", "defaults", "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Operations = append(cfg.Operations, config.OperationDefinition{Name: "group", RunnerProfile: config.ProfileServiceManager, ExecutorKind: config.ExecutorSystemd, Systemd: &config.SystemdOperationDefinition{Units: []string{"secret-db.service", "secret-web.service"}, Action: "restart", Alias: "application"}})
	lines, err := CatalogSudoers(cfg)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")
	for _, unit := range []string{"secret-db.service", "secret-web.service"} {
		for _, action := range []string{"start", "stop"} {
			if !strings.Contains(text, "/usr/bin/systemctl --no-pager "+action+" "+unit) {
				t.Fatalf("missing exact pair: %s %s", action, unit)
			}
		}
		for _, action := range []string{"restart", "status", "enable", "disable"} {
			if strings.Contains(text, action+" "+unit) {
				t.Fatalf("unnecessary grant: %s %s", action, unit)
			}
		}
	}
	if strings.ContainsAny(text, "*?;") {
		t.Fatal("nonliteral sudoers grant")
	}
}
