package config

import (
	"path/filepath"
	"testing"
)

// Keep the checked-in operator catalog executable as a tested artifact. This
// catches a missing executable binding, policy entry, or malformed typed
// systemd operation before a package is built.
func TestShippedCatalogIncludesDiagnosticsAndServiceOperations(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load shipped catalog: %v", err)
	}
	operations := make(map[string]OperationDefinition, len(cfg.Operations))
	for _, operation := range cfg.Operations {
		operations[operation.Name] = operation
	}
	for _, name := range []string{
		"ping",
		"status_safeterm_audit", "status_safeterm_gateway", "status_safeterm_ssh",
		"restart_safeterm_audit", "restart_safeterm_gateway", "restart_safeterm_ssh",
	} {
		if _, ok := operations[name]; !ok {
			t.Errorf("shipped catalog is missing %q", name)
		}
	}

	ping := operations["ping"]
	if ping.ExecutableRef != "ping" || len(ping.Parameters) != 2 || ping.Parameters[0].Name != "host" || ping.Parameters[1].Default != "100" {
		t.Fatalf("ping definition is not the expected bounded host operation: %+v", ping)
	}
	if len(ping.ArgsTemplate) == 0 || ping.ArgsTemplate[len(ping.ArgsTemplate)-1] != "{host}" {
		t.Fatalf("ping must pass the declared host as its final argument: %+v", ping.ArgsTemplate)
	}
}
