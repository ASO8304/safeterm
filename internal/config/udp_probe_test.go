package config

import "testing"

func TestUDPProbeRequiresBoundedNetcatAndNetworkProfile(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*OperationDefinition)
	}{
		{"timeout", func(o *OperationDefinition) { o.UDPProbe.TimeoutSeconds = 61 }},
		{"missing-type", func(o *OperationDefinition) { o.UDPProbe = nil }},
		{"wrong-tool", func(o *OperationDefinition) { o.ExecutableRef = "ping" }},
		{"templates", func(o *OperationDefinition) { o.ArgsTemplate = []string{"--script=all"} }},
		{"wrong-profile", func(o *OperationDefinition) { o.RunnerProfile = ProfileDiagnostics }},
		{"not-streaming", func(o *OperationDefinition) { o.LongRunning = false }},
		{"unbounded-port", func(o *OperationDefinition) { o.Parameters[1].Max = nil }},
		{"optional-port", func(o *OperationDefinition) { o.Parameters[1].Required = false }},
		{"mixed-definitions", func(o *OperationDefinition) { o.TCPProbe = &TCPProbeDefinition{TimeoutSeconds: 5} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadConfig("../defaults/catalog")
			if err != nil {
				t.Fatal(err)
			}
			for i := range cfg.Operations {
				if cfg.Operations[i].Name == "udp-test" {
					tc.mutate(&cfg.Operations[i])
				}
			}
			if ValidateConfig(cfg) == nil {
				t.Fatal("unsafe UDP definition accepted")
			}
		})
	}
}
