package translation

import (
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/validation"
)

func TestTraceDynamicFilters(t *testing.T) {
	cfg := shippedConfig(t)
	tr := mustTranslator(t, cfg)
	validator := validation.New(cfg)
	for _, tc := range []struct {
		name   string
		values map[string]string
		filter []string
	}{
		{"host", map[string]string{"host": "192.0.2.1"}, []string{"host", "192.0.2.1"}},
		{"port", map[string]string{"port": "999"}, []string{"port", "999"}},
		{"both", map[string]string{"host": "192.0.2.1", "port": "999"}, []string{"port", "999", "and", "host", "192.0.2.1"}},
		{"host protocol", map[string]string{"host": "192.0.2.1", "protocol": "icmp"}, []string{"host", "192.0.2.1", "and", "icmp"}},
		{"port protocol", map[string]string{"port": "999", "protocol": "udp"}, []string{"port", "999", "and", "udp"}},
		{"all", map[string]string{"host": "192.0.2.1", "port": "999", "protocol": "tcp"}, []string{"port", "999", "and", "host", "192.0.2.1", "and", "tcp"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &contract.ExecutionRequest{Command: "trace", Arguments: tc.values}
			if err := validator.Validate(req); err != nil {
				t.Fatal(err)
			}
			assertTranslation(t, tr, req, "/usr/bin/tcpdump", append([]string{"-l", "-nn", "-i", "any"}, tc.filter...))
		})
	}
	for _, values := range []map[string]string{nil, {"protocol": "tcp"}, {"host": ""}} {
		req := &contract.ExecutionRequest{Command: "trace", Arguments: values}
		if err := validator.Validate(req); err == nil || !strings.Contains(err.Error(), "host or port must be specified") {
			t.Fatalf("%v: %v", values, err)
		}
		assertRejected(t, tr, req, "host or port must be specified")
	}
	for _, values := range []map[string]string{
		{"host": "any"}, {"port": "any"}, {"host": "any", "port": "999"}, {"host": "192.0.2.1", "protocol": "any"},
		{"port": "999", "protocol": "tcp or udp"}, {"port": "0"}, {"port": "65536"}, {"host": "192.0.2.1;id"}, {"port": "999", "unknown": "x"},
	} {
		if err := validator.Validate(&contract.ExecutionRequest{Command: "trace", Arguments: values}); err == nil {
			t.Fatalf("accepted %v", values)
		}
	}
}

func TestArgumentGroupsValidateAndSnapshot(t *testing.T) {
	cfg := tokenConfig("--fixed")
	cfg.Operations[0].ArgumentGroups = []config.ArgumentGroup{{When: "value", ArgsTemplate: []string{"--value", "{value}"}}}
	cfg.Operations[0].RequiredAny = []string{"value"}
	tr := mustTranslator(t, cfg)
	cfg.Operations[0].ArgumentGroups[0].ArgsTemplate[0] = "changed"
	cfg.Operations[0].RequiredAny[0] = "changed"
	req := &contract.ExecutionRequest{Command: "example", Arguments: map[string]string{"value": "hello"}}
	assertTranslation(t, tr, req, "/usr/bin/example", []string{"--fixed", "--value", "hello"})
	if !reflect.DeepEqual(req.Arguments, map[string]string{"value": "hello"}) {
		t.Fatal("request mutated")
	}
	for _, group := range []config.ArgumentGroup{{When: "missing", ArgsTemplate: []string{"x"}}, {When: "value"}, {When: "value", ArgsTemplate: []string{"{missing}"}}, {When: "value", ArgsTemplate: []string{"x{value}"}}} {
		cfg = tokenConfig("fixed")
		cfg.Operations[0].ArgumentGroups = []config.ArgumentGroup{group}
		if _, err := New(cfg); err == nil {
			t.Fatalf("accepted invalid group %+v", group)
		}
	}
	cfg = tokenConfig("fixed")
	cfg.Operations[0].RequiredAny = []string{"missing"}
	if _, err := New(cfg); err == nil {
		t.Fatal("undefined required_any")
	}
	cfg = tokenConfig("fixed")
	cfg.Operations[0].RequiredAny = []string{"value", "value"}
	if _, err := New(cfg); err == nil {
		t.Fatal("duplicate required_any")
	}
	cfg = tokenConfig("fixed")
	cfg.Operations[0].RequiredAny = []string{"value"}
	cfg.Operations[0].Parameters[0].Default = "default"
	cfg.Operations[0].ArgumentGroups = []config.ArgumentGroup{{When: "value", ArgsTemplate: []string{"{value}"}}}
	req = &contract.ExecutionRequest{Command: "example"}
	if err := validation.New(cfg).Validate(req); err != nil {
		t.Fatal(err)
	}
	assertTranslation(t, mustTranslator(t, cfg), req, "/usr/bin/example", []string{"fixed", "default"})
}
