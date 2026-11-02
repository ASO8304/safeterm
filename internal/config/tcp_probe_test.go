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

func TestLegacyNativeTCPRegistryRemainsReadableButCannotDeployWithoutNetcat(t *testing.T) {
	cfg, err := LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	ops := cfg.Operations[:0]
	for _, op := range cfg.Operations {
		if op.Name == "udp-test" {
			continue
		}
		if op.Name == "telnet" {
			op.ExecutableRef = ""
		}
		ops = append(ops, op)
	}
	cfg.Operations = ops
	executables := cfg.Executables[:0]
	for _, executable := range cfg.Executables {
		if executable.ID != "netcat" {
			executables = append(executables, executable)
		}
	}
	cfg.Executables = executables
	for i := range cfg.Policies {
		operations := []string{}
		for _, operation := range cfg.Policies[i].Operations {
			if operation != "udp-test" {
				operations = append(operations, operation)
			}
		}
		cfg.Policies[i].Operations = operations
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatal("legacy catalog cannot be verified for upgrade", err)
	}
	if err := ValidateRuntimeProbes(cfg); err == nil {
		t.Fatal("legacy native-only definition was considered deployable")
	}
}
