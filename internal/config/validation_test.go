package config

import (
	"strings"
	"testing"
)

const testSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"

func intPtr(v int) *int { return &v }

func validOperation() OperationDefinition {
	return OperationDefinition{Name: "system_info", RunnerProfile: ProfileDiagnostics, ExecutorKind: ExecutorProcess, ExecutableRef: "uname"}
}

func validConfig() *Config {
	return &Config{
		Version: "test-v1",
		Profiles: []ProfileDefinition{
			{Name: ProfileDiagnostics, ExecutorKind: ExecutorProcess, SocketPath: "/run/safeterm/diagnostics.sock", RunAsUser: "safeterm-diagnostics", MemoryLimitMiB: 128, ProcessLimit: 32},
			{Name: ProfileNetworkObserver, ExecutorKind: ExecutorProcess, SocketPath: "/run/safeterm/network.sock", RunAsUser: "safeterm-network", Capabilities: []string{"CAP_NET_RAW"}, MemoryLimitMiB: 256, ProcessLimit: 32},
			{Name: ProfileServiceManager, ExecutorKind: ExecutorSystemd, ControllerExecutableRef: "systemctl", SocketPath: "/run/safeterm/service.sock", RunAsUser: "safeterm-service", MemoryLimitMiB: 128, ProcessLimit: 16},
		},
		Executables: []ExecutableDefinition{
			{ID: "uname", Path: "/usr/bin/uname", SHA256: testSHA256, AllowedProfiles: []RunnerProfile{ProfileDiagnostics}},
			{ID: "systemctl", Path: "/usr/bin/systemctl", SHA256: testSHA256, AllowedProfiles: []RunnerProfile{ProfileServiceManager}},
		},
		Operations: []OperationDefinition{validOperation()},
		Roles:      []RoleDefinition{{Name: "viewer"}},
		Policies:   []PolicyDefinition{{Role: "viewer", Operations: []string{"system_info"}}},
	}
}

func requireInvalid(t *testing.T, cfg *Config, contains string) {
	t.Helper()
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("ValidateConfig() = %v, want error containing %q", err, contains)
	}
}

// TEST-CONFIG-001 / TEST-ISOLATION-001
func TestValidateConfig_ValidProfileMappedConfigPasses(t *testing.T) {
	if err := ValidateConfig(validConfig()); err != nil {
		t.Fatal(err)
	}
}

// TEST-CONFIG-002
func TestValidateConfig_DuplicateOperationNameRejected(t *testing.T) {
	cfg := validConfig()
	cfg.Operations = append(cfg.Operations, validOperation())
	requireInvalid(t, cfg, "duplicate operation")
}

// TEST-CONFIG-003 / TEST-ISOLATION-001
func TestValidateConfig_MissingOrConflictingProfileMappingRejected(t *testing.T) {
	for _, name := range []string{"missing profile", "unknown profile", "kind conflict"} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			switch name {
			case "missing profile":
				cfg.Operations[0].RunnerProfile = ""
			case "unknown profile":
				cfg.Operations[0].RunnerProfile = "ghost"
			case "kind conflict":
				cfg.Operations[0].ExecutorKind = ExecutorSystemd
			}
			requireInvalid(t, cfg, "operation")
		})
	}
}

// TEST-CONFIG-004 through TEST-CONFIG-006
func TestValidateConfig_ReferentialIntegrityRejected(t *testing.T) {
	for _, name := range []string{"duplicate role", "unknown role", "unknown operation"} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			switch name {
			case "duplicate role":
				cfg.Roles = append(cfg.Roles, cfg.Roles[0])
			case "unknown role":
				cfg.Policies[0].Role = "ghost"
			case "unknown operation":
				cfg.Policies[0].Operations[0] = "ghost"
			}
			requireInvalid(t, cfg, "role")
		})
	}
}

// TEST-CONFIG-007 through TEST-CONFIG-010
func TestValidateConfig_ParameterDefinitionsRejected(t *testing.T) {
	for _, name := range []string{"int pattern", "min max", "string range", "unknown type"} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			var parameter ParameterDefinition
			switch name {
			case "int pattern":
				parameter = ParameterDefinition{Name: "port", Type: ParamTypeInt, Pattern: "x"}
			case "min max":
				parameter = ParameterDefinition{Name: "port", Type: ParamTypeInt, Min: intPtr(2), Max: intPtr(1)}
			case "string range":
				parameter = ParameterDefinition{Name: "host", Type: ParamTypeString, Min: intPtr(1)}
			case "unknown type":
				parameter = ParameterDefinition{Name: "value", Type: "float"}
			}
			cfg.Operations[0].Parameters = []ParameterDefinition{parameter}
			requireInvalid(t, cfg, "parameter")
		})
	}
}

// TEST-ISOLATION-002, TEST-ISOLATION-005
func TestValidateConfig_ProfileSecurityContractRejected(t *testing.T) {
	for _, name := range []string{"duplicate", "kind", "socket", "user", "memory", "process", "capability", "sys admin"} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			switch name {
			case "duplicate":
				cfg.Profiles = append(cfg.Profiles, cfg.Profiles[0])
			case "kind":
				cfg.Profiles[0].ExecutorKind = "shell"
			case "socket":
				cfg.Profiles[0].SocketPath = "relative.sock"
			case "user":
				cfg.Profiles[0].RunAsUser = ""
			case "memory":
				cfg.Profiles[0].MemoryLimitMiB = 0
			case "process":
				cfg.Profiles[0].ProcessLimit = 0
			case "capability":
				cfg.Profiles[0].Capabilities = []string{"CAP_NET_RAW", "CAP_NET_RAW"}
			case "sys admin":
				cfg.Profiles[0].Capabilities = []string{"CAP_SYS_ADMIN"}
			}
			requireInvalid(t, cfg, "profile")
		})
	}
}

// TEST-ISOLATION-007, TEST-ISOLATION-008
func TestValidateConfig_TypedBackendsRejectFreeFormConfiguration(t *testing.T) {
	for _, name := range []string{"unknown executable", "cross profile executable", "systemd executable", "systemd arguments", "systemd parameters", "bad unit", "bad action"} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			switch name {
			case "unknown executable":
				cfg.Operations[0].ExecutableRef = "shell"
			case "cross profile executable":
				cfg.Operations[0].RunnerProfile = ProfileNetworkObserver
			case "systemd executable", "systemd arguments", "systemd parameters", "bad unit", "bad action":
				cfg.Operations[0] = OperationDefinition{Name: "restart_demo", RunnerProfile: ProfileServiceManager, ExecutorKind: ExecutorSystemd, Systemd: &SystemdOperationDefinition{Unit: "demo.service", Action: "restart"}}
				cfg.Policies[0].Operations[0] = "restart_demo"
				if name == "systemd executable" {
					cfg.Operations[0].ExecutableRef = "uname"
				}
				if name == "systemd arguments" {
					cfg.Operations[0].ArgsTemplate = []string{"--all"}
				}
				if name == "systemd parameters" {
					cfg.Operations[0].Parameters = []ParameterDefinition{{Name: "unit", Type: ParamTypeString}}
				}
				if name == "bad unit" {
					cfg.Operations[0].Systemd.Unit = "../../demo"
				}
				if name == "bad action" {
					cfg.Operations[0].Systemd.Action = "enable"
				}
			}
			requireInvalid(t, cfg, "operation")
		})
	}
}

// TEST-INTEGRITY-016
func TestValidateConfig_ExecutableRegistryRejected(t *testing.T) {
	for _, name := range []string{"duplicate", "relative path", "unclean path", "duplicate path", "missing digest", "uppercase digest", "no profiles", "unknown profile", "missing controller", "cross profile controller"} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			switch name {
			case "duplicate":
				cfg.Executables = append(cfg.Executables, cfg.Executables[0])
			case "relative path":
				cfg.Executables[0].Path = "uname"
			case "unclean path":
				cfg.Executables[0].Path = "/usr/bin/../bin/uname"
			case "duplicate path":
				cfg.Executables[1].Path = cfg.Executables[0].Path
			case "missing digest":
				cfg.Executables[0].SHA256 = ""
			case "uppercase digest":
				cfg.Executables[0].SHA256 = strings.ToUpper(testSHA256[:63] + "a")
			case "no profiles":
				cfg.Executables[0].AllowedProfiles = nil
			case "unknown profile":
				cfg.Executables[0].AllowedProfiles = []RunnerProfile{"ghost"}
			case "missing controller":
				cfg.Profiles[2].ControllerExecutableRef = ""
			case "cross profile controller":
				cfg.Profiles[2].ControllerExecutableRef = "uname"
			}
			requireInvalid(t, cfg, "executable")
		})
	}
}
