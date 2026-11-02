package main

import (
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/siteconfig"
)

func TestTCPDefaultAliasKeepsSignedPermissionIdentity(t *testing.T) {
	cfg, err := config.LoadConfig("../../internal/defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tcp-test", "telnet"} {
		value, err := defaultDefinition(cfg, "operation", name)
		if err != nil {
			t.Fatal(err)
		}
		op, ok := value.(siteconfig.Operation)
		if !ok || op.Operation.Name != "telnet" || op.Operation.ExecutorKind != config.ExecutorTCPProbe {
			t.Fatal("alias changed signed identity", value)
		}
	}
}
