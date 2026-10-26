package config

import "testing"

func TestTCPProbeDefinitionIsBoundedAndProfileScoped(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*OperationDefinition)
	}{
		{"timeout", func(o *OperationDefinition) { o.TCPProbe.TimeoutSeconds = 61 }},
		{"missing-type", func(o *OperationDefinition) { o.TCPProbe = nil }},
		{"process", func(o *OperationDefinition) { o.ExecutableRef = "ping" }},
		{"templates", func(o *OperationDefinition) { o.ArgsTemplate = []string{"sh"} }},
		{"wrong-profile", func(o *OperationDefinition) { o.RunnerProfile = ProfileDiagnostics }},
		{"not-streaming", func(o *OperationDefinition) { o.LongRunning = false }},
		{"unbounded-port", func(o *OperationDefinition) { o.Parameters[1].Max = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadConfig("../defaults/catalog")
			if err != nil {
				t.Fatal(err)
			}
			for i := range cfg.Operations {
				if cfg.Operations[i].Name == "telnet" {
					tc.mutate(&cfg.Operations[i])
					break
				}
			}
			if ValidateConfig(cfg) == nil {
				t.Fatal("unsafe probe config accepted")
			}
		})
	}
}
