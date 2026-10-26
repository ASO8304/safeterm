package translation

import (
	"reflect"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"testing"
)

func TestTCPProbeTranslationUsesTypedOperationAndDefaultPort(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	tr, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		arguments map[string]string
		port      string
	}{{map[string]string{"host": "127.0.0.1"}, "23"}, {map[string]string{"host": "127.0.0.1", "port": "443"}, "443"}} {
		req, _ := contract.NewExecutionRequest("telnet", tc.arguments, contract.Metadata{})
		command, args, err := tr.Translate(req)
		if err != nil || command != "telnet" || !reflect.DeepEqual(args, []string{"127.0.0.1", tc.port}) {
			t.Fatal(command, args, err)
		}
		if _, exists := tc.arguments["port"]; !exists && len(tc.arguments) != 1 {
			t.Fatal("translation mutated caller")
		}
	}
}
