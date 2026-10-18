package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestShippedTcpdumpOperationsFlushOutputWithoutNameResolution(t *testing.T) {
	cfg, err := LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, op := range cfg.Operations {
		if op.ExecutableRef != "tcpdump" {
			continue
		}
		count++
		if !op.LongRunning || !slices.Contains(op.ArgsTemplate, "-l") || !slices.Contains(op.ArgsTemplate, "-nn") {
			t.Errorf("%s: tcpdump must stream line-buffered numeric output", op.Name)
		}
	}
	if count != 1 {
		t.Fatalf("expected one capture operation, found %d", count)
	}
}

func TestShippedTracerouteUsesStreaming(t *testing.T) {
	cfg, err := LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range cfg.Operations {
		if op.Name == "traceroute" {
			if !op.LongRunning || op.RunnerProfile != ProfileNetworkObserver {
				t.Fatal("traceroute must use the network observer streaming path")
			}
			if !slices.Contains(op.ArgsTemplate, "-n") {
				t.Fatal("traceroute must avoid reverse DNS delays")
			}
			return
		}
	}
	t.Fatal("traceroute operation missing")
}

func TestShippedPingUsesStreaming(t *testing.T) {
	cfg, err := LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range cfg.Operations {
		if op.Name == "ping" {
			if !op.LongRunning {
				t.Fatal("ping must stream output before process exit")
			}
			return
		}
	}
	t.Fatal("ping operation missing")
}

// writeConfigDir creates <base>/operations, <base>/roles and <base>/policies
// under a fresh temp directory and writes the given JSON content into a
// single file in each. Passing an empty string for any of them results in
// that folder being created but left empty, so LoadConfig sees zero
// definitions for that category rather than an error.
func writeConfigDir(t *testing.T, operationsJSON, rolesJSON, policiesJSON string) string {
	t.Helper()
	base := t.TempDir()

	write := func(sub, filename, content string) {
		dir := filepath.Join(base, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", dir, err)
		}
		if content == "" {
			content = "[]"
		}
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", filename, err)
		}
	}

	write("operations", "operations.json", operationsJSON)
	write("executables", "executables.json", `[{"id":"uname","path":"/usr/bin/uname","sha256":"0000000000000000000000000000000000000000000000000000000000000000","allowed_profiles":["diagnostics"]},{"id":"tcpdump","path":"/usr/bin/tcpdump","sha256":"0000000000000000000000000000000000000000000000000000000000000000","allowed_profiles":["network-observer"]}]`)
	write("profiles", "profiles.json", `[{"name":"diagnostics","executor_kind":"process","socket_path":"/run/safeterm/diagnostics.sock","run_as_user":"safeterm-diagnostics","capabilities":[],"memory_limit_mib":128,"process_limit":32},{"name":"network-observer","executor_kind":"process","socket_path":"/run/safeterm/network.sock","run_as_user":"safeterm-network","capabilities":["CAP_NET_RAW"],"memory_limit_mib":256,"process_limit":32}]`)
	write("roles", "roles.json", rolesJSON)
	write("policies", "policies.json", policiesJSON)
	if err := os.WriteFile(filepath.Join(base, "catalog.json"), []byte(`{"version":"test-v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	return base
}

// TEST-CONFIG-011
// Requirement: REQ-003 (AC-7)
// Scenario: LoadConfig must successfully parse a well-formed set of JSON
// files from disk and return a Config that passes validation.
func TestLoadConfig_WellFormedFiles_Succeeds(t *testing.T) {
	dir := writeConfigDir(t,
		`[{"name":"system_info","runner_profile":"diagnostics","executor_kind":"process","executable_ref":"uname"}]`,
		`[{"name":"viewer"}]`,
		`[{"role":"viewer","operations":["system_info"]}]`,
	)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("expected LoadConfig to succeed, got error: %v", err)
	}
	if len(cfg.Operations) != 1 || cfg.Operations[0].Name != "system_info" {
		t.Fatalf("expected one operation named system_info, got %+v", cfg.Operations)
	}
}

// TEST-CONFIG-012
// Requirement: REQ-003 (AC-8)
// Scenario: LoadConfig must return an error (not panic, not a partial
// Config) when a required directory does not exist.
func TestLoadConfig_MissingDirectory_ReturnsError(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing config directory, got nil")
	}
}

// TEST-CONFIG-013
// Requirement: REQ-003 (AC-9)
// Scenario: LoadConfig must return an error when a JSON file is malformed,
// rather than silently loading an empty/partial list.
func TestLoadConfig_MalformedJSON_ReturnsError(t *testing.T) {
	dir := writeConfigDir(t, `{ this is not valid json`, "", "")

	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// TEST-CONFIG-014
// Requirement: REQ-003 (AC-10)
// Scenario: LoadConfig must propagate a ValidateConfig failure (e.g. a
// dangling policy reference) as an error, and must not return the
// inconsistent Config to the caller.
func TestLoadConfig_InconsistentConfig_ReturnsErrorNotPartialConfig(t *testing.T) {
	dir := writeConfigDir(t,
		`[{"name":"system_info","runner_profile":"diagnostics","executor_kind":"process","executable_ref":"uname"}]`,
		`[{"name":"viewer"}]`,
		`[{"role":"viewer","operations":["ghost_operation"]}]`,
	)

	cfg, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("expected error for policy referencing unknown operation, got nil")
	}
	if cfg != nil {
		t.Fatal("expected LoadConfig to return a nil Config on validation failure")
	}
}

// TEST-CONFIG-015
// Requirement: REQ-003 (AC-11)
// Scenario: multiple *.json files inside the same folder (e.g.
// operations/) must be merged together, so adding a new operation only
// requires dropping in a new file, never editing an existing one.
func TestLoadConfig_MultipleFilesInSameFolder_AreMerged(t *testing.T) {
	base := t.TempDir()
	opsDir := filepath.Join(base, "operations")
	if err := os.MkdirAll(opsDir, 0o755); err != nil {
		t.Fatalf("failed to create operations dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(opsDir, "core.json"), []byte(`[{"name":"system_info","runner_profile":"diagnostics","executor_kind":"process","executable_ref":"uname"}]`), 0o644); err != nil {
		t.Fatalf("failed to write core.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(opsDir, "network.json"), []byte(`[{"name":"trace","runner_profile":"network-observer","executor_kind":"process","executable_ref":"tcpdump"}]`), 0o644); err != nil {
		t.Fatalf("failed to write network.json: %v", err)
	}
	for _, sub := range []string{"roles", "policies"} {
		if err := os.MkdirAll(filepath.Join(base, sub), 0o755); err != nil {
			t.Fatalf("failed to create %s dir: %v", sub, err)
		}
		if err := os.WriteFile(filepath.Join(base, sub, "empty.json"), []byte("[]"), 0o644); err != nil {
			t.Fatalf("failed to write empty %s file: %v", sub, err)
		}
	}
	writeArray := func(sub, content string) {
		if err := os.MkdirAll(filepath.Join(base, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, sub, sub+".json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeArray("executables", `[{"id":"uname","path":"/usr/bin/uname","sha256":"0000000000000000000000000000000000000000000000000000000000000000","allowed_profiles":["diagnostics"]},{"id":"tcpdump","path":"/usr/bin/tcpdump","sha256":"0000000000000000000000000000000000000000000000000000000000000000","allowed_profiles":["network-observer"]}]`)
	writeArray("profiles", `[{"name":"diagnostics","executor_kind":"process","socket_path":"/run/safeterm/diagnostics.sock","run_as_user":"safeterm-diagnostics","memory_limit_mib":128,"process_limit":32},{"name":"network-observer","executor_kind":"process","socket_path":"/run/safeterm/network.sock","run_as_user":"safeterm-network","memory_limit_mib":128,"process_limit":32}]`)
	if err := os.WriteFile(filepath.Join(base, "catalog.json"), []byte(`{"version":"test-v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(base)
	if err != nil {
		t.Fatalf("expected LoadConfig to succeed, got error: %v", err)
	}
	if len(cfg.Operations) != 2 {
		t.Fatalf("expected 2 merged operations from 2 files, got %d", len(cfg.Operations))
	}
}

// TEST-CONFIG-016
// Scenario: the actual configuration files shipped in the repository
// (configs/operations, configs/roles, configs/policies) must themselves
// load and validate successfully. This is a regression guard: if someone
// hand-edits a shipped config file into an inconsistent state, this test
// fails in CI before it ever reaches a real deployment.
func TestLoadConfig_ShippedRepositoryConfig_IsValid(t *testing.T) {
	cfg, err := LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatalf("expected shipped repository config to load and validate, got error: %v", err)
	}
	if len(cfg.Operations) == 0 {
		t.Fatal("expected the shipped config to define at least one operation")
	}
}
