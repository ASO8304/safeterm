package validation

import (
	"safeterm/internal/config"
	"testing"
)

func TestPingHardLimitIsEnforcedWithoutCLI(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	for i := range cfg.Operations {
		if cfg.Operations[i].Name == "ping" {
			for j := range cfg.Operations[i].Parameters {
				p := &cfg.Operations[i].Parameters[j]
				if p.Name == "count" {
					maximum := 5000
					p.Max = &maximum
				}
			}
		}
	}
	validator := New(cfg)
	for _, count := range []string{"0", "1001", "1100", "5000", "-1", "1;id"} {
		if err := validator.Validate(newRequest(t, "ping", map[string]string{"host": "127.0.0.1", "count": count})); err == nil {
			t.Fatal("count bypassed limit", count)
		}
	}
	for _, count := range []string{"1", "1000"} {
		if err := validator.Validate(newRequest(t, "ping", map[string]string{"host": "127.0.0.1", "count": count})); err != nil {
			t.Fatal(err)
		}
	}
}
func TestTCPProbeRejectsHostnamesAndInvalidTargetsBeforeExecution(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	v := New(cfg)
	for _, host := range []string{"example.com", "127.0.0.1;id", "0.0.0.0", "224.0.0.1", "::", "fe80::1%eth0"} {
		if err := v.Validate(newRequest(t, "telnet", map[string]string{"host": host})); err == nil {
			t.Fatal("accepted", host)
		}
	}
	for _, port := range []string{"0", "65536", "23;id"} {
		if err := v.Validate(newRequest(t, "telnet", map[string]string{"host": "127.0.0.1", "port": port})); err == nil {
			t.Fatal("accepted", port)
		}
	}
	for _, host := range []string{"127.0.0.1", "::1"} {
		if err := v.Validate(newRequest(t, "telnet", map[string]string{"host": host})); err != nil {
			t.Fatal(err)
		}
	}
}
