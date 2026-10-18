//go:build linux

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/deployment"
	"safeterm/internal/integrity"
)

type catalogFakeHost struct {
	active map[string]bool
	calls  []string
	fail   func(string, []string) bool
}

func TestPortableCatalogAppliesWithTargetTrustAndExplicitHashRefresh(t *testing.T) {
	r, _, _ := catalogFixture(t)
	portable := filepath.Join(t.TempDir(), "portable")
	if err := deployment.PrepareConfigWithHashes(r.live, portable, false); err != nil {
		t.Fatal(err)
	}
	catalogAddAlias(t, portable)
	cfg, err := config.LoadConfig(portable)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Executables[0].SHA256 = strings.Repeat("0", 64) // A different host's executable digest.
	if err := catalogWriteModel(portable, cfg); err != nil {
		t.Fatal(err)
	}
	if err := r.apply(portable, false, true); err == nil {
		t.Fatal("ordinary apply implicitly refreshed hashes")
	}
	r.refreshHashes = true
	if err := r.apply(portable, false, true); err != nil {
		t.Fatal(err)
	}
	if catalogHasAlias(t, r) {
		t.Fatal("dry run changed live catalog")
	}
	if err := r.apply(portable, false, false); err != nil {
		t.Fatal(err)
	}
	if !catalogHasAlias(t, r) {
		t.Fatal("portable log alias was not applied")
	}
	if err := r.rollback(); err != nil {
		t.Fatal(err)
	}
	if catalogHasAlias(t, r) {
		t.Fatal("portable import did not retain rollback baseline")
	}
}

func (h *catalogFakeHost) run(name string, args ...string) ([]byte, error) {
	h.calls = append(h.calls, filepath.Base(name)+" "+strings.Join(args, " "))
	if h.fail != nil && h.fail(name, args) {
		return nil, errors.New("injected failure")
	}
	if filepath.Base(name) != "systemctl" {
		return nil, nil
	}
	unit := args[len(args)-1]
	switch args[0] {
	case "is-active":
		if h.active[unit] {
			return []byte("active\n"), nil
		}
		return []byte("inactive\n"), errors.New("inactive")
	case "stop":
		h.active[unit] = false
	case "start":
		h.active[unit] = true
	}
	return nil, nil
}

func catalogFixture(t *testing.T) (*catalogRuntime, *catalogFakeHost, ed25519.PrivateKey) {
	t.Helper()
	dir := t.TempDir()
	live := filepath.Join(dir, "live")
	state := filepath.Join(dir, "state")
	if err := os.MkdirAll(state, 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executableData, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(executableData)
	cfg := &config.Config{Version: "catalog-test", Profiles: []config.ProfileDefinition{
		{Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/safeterm-diagnostics/runner.sock", RunAsUser: "safeterm-diagnostics", MemoryLimitMiB: 64, ProcessLimit: 8},
		{Name: config.ProfileServiceManager, ExecutorKind: config.ExecutorSystemd, ControllerExecutableRef: "native", SocketPath: "/run/safeterm-service-manager/runner.sock", RunAsUser: "safeterm-service-manager", MemoryLimitMiB: 64, ProcessLimit: 8}},
		Executables: []config.ExecutableDefinition{{ID: "native", Path: executable, SHA256: hex.EncodeToString(digest[:]), AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics, config.ProfileServiceManager}}},
		Operations: []config.OperationDefinition{
			{Name: "log", Description: "Test log", RunnerProfile: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, ExecutableRef: "native", ArgsTemplate: []string{"-F", "--", "{source}"}, LongRunning: true, Parameters: []config.ParameterDefinition{{Name: "source", Type: "string", Required: true, ValueMap: map[string]string{"old": "/var/log/old.log"}}}},
			{Name: "status_demo", Description: "Test status", RunnerProfile: config.ProfileServiceManager, ExecutorKind: config.ExecutorSystemd, Systemd: &config.SystemdOperationDefinition{Unit: "demo.service", Action: "status"}}},
		Roles: []config.RoleDefinition{{Name: "viewer", Description: "View"}}, Policies: []config.PolicyDefinition{{Role: "viewer", Operations: []string{"log", "status_demo"}}}}
	if err = catalogWriteModel(live, cfg); err != nil {
		t.Fatal(err)
	}
	public, key, _ := ed25519.GenerateKey(rand.Reader)
	for _, section := range []string{"trust", "ssh", "console", "config-manager"} {
		if err = os.MkdirAll(filepath.Join(live, section), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(live, "trust/catalog-ed25519.pub"), integrity.EncodeBase64(public), 0644); err != nil {
		t.Fatal(err)
	}
	signature, err := integrity.SignCatalog(cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(live, "catalog.sig"), integrity.EncodeBase64(signature), 0644); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(dir, "key")
	if err = os.WriteFile(keyFile, integrity.EncodeBase64(key), 0600); err != nil {
		t.Fatal(err)
	}
	sshKey, _ := ssh.NewPublicKey(public)
	write := func(name string, data any, mode os.FileMode) {
		t.Helper()
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(live, name), b, mode); err != nil {
			t.Fatal(err)
		}
	}
	write("ssh/users.json", map[string]any{"users": []any{map[string]any{"user_id": "alice", "roles": []string{"viewer"}, "authorized_keys": []string{string(ssh.MarshalAuthorizedKey(sshKey))}}}}, 0600)
	write("console/users.json", map[string]any{"users": []any{map[string]any{"user_id": "alice", "roles": []string{"viewer"}, "disabled": true}}}, 0600)
	registry := configmanager.Registry{Version: "test", Resources: []configmanager.Resource{{ID: "demo-options", Description: "Test", Profile: "application", Path: "/etc/safeterm-managed/application/options.json", Format: configmanager.FormatJSON, Access: configmanager.AccessReadOnly, Fields: []configmanager.FieldSchema{{Key: "enabled", Type: configmanager.FieldBoolean, Required: true}}, Permissions: configmanager.Permissions{ReadRoles: []string{"viewer"}}}}}
	write("resources.json", registry, 0644)
	host := &catalogFakeHost{active: map[string]bool{serviceUnits["identity"]: true, serviceUnits["logs"]: true, serviceUnits["gateway"]: true}}
	var out bytes.Buffer
	r := &catalogRuntime{live: live, state: state, key: keyFile, sudoers: filepath.Join(dir, "sudoers"), out: &out, run: host.run, registry: func(path string) (*configmanager.Registry, error) {
		b, err := catalogRead(path)
		if err != nil {
			return nil, err
		}
		return configmanager.ParseRegistry(b)
	}}
	if err = r.checkpoint(); err != nil {
		t.Fatal(err)
	}
	// Initialize deployed sudoers as a real installation would.
	b, err := os.ReadFile(filepath.Join(state, "current/sudoers"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(r.sudoers, b, 0440); err != nil {
		t.Fatal(err)
	}
	host.calls = nil
	return r, host, key
}
func catalogAddAlias(t *testing.T, dir string) {
	t.Helper()
	cfg, err := config.LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Operations[0].Parameters[0].ValueMap["myapp"] = "/var/log/myapp/app.log"
	if err = catalogWriteModel(dir, cfg); err != nil {
		t.Fatal(err)
	}
}
func catalogHasAlias(t *testing.T, r *catalogRuntime) bool {
	t.Helper()
	cfg, err := r.verified(r.live)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Operations[0].Parameters[0].ValueMap["myapp"] != ""
}

func TestCatalogApplyRepairsEditedLiveAndRollbackPreservesOtherFiles(t *testing.T) {
	r, host, _ := catalogFixture(t)
	before := map[string][]byte{}
	for _, name := range []string{"ssh/users.json", "console/users.json", "resources.json", "trust/catalog-ed25519.pub"} {
		before[name], _ = os.ReadFile(filepath.Join(r.live, name))
	}
	catalogAddAlias(t, r.live)
	if _, err := r.verified(r.live); err == nil {
		t.Fatal("fixture signature should be stale")
	}
	if err := r.apply(r.live, false, false); err != nil {
		t.Fatal(err)
	}
	if !catalogHasAlias(t, r) {
		t.Fatal("alias was not deployed")
	}
	calls := strings.Join(host.calls, "\n")
	if strings.Index(calls, "stop "+serviceUnits["gateway"]) > strings.Index(calls, "stop "+serviceUnits["identity"]) {
		t.Fatal("dependents must stop before Identity")
	}
	if strings.Contains(calls, "stop "+serviceUnits["audit"]) {
		t.Fatal("catalog activation stopped durable audit")
	}
	for name, want := range before {
		got, _ := os.ReadFile(filepath.Join(r.live, name))
		if !bytes.Equal(want, got) {
			t.Fatalf("non-catalog file changed: %s", name)
		}
	}
	if err := r.rollback(); err != nil {
		t.Fatal(err)
	}
	if catalogHasAlias(t, r) {
		t.Fatal("rollback did not restore the earlier catalog")
	}
	for _, name := range []string{"identity", "logs", "gateway"} {
		if !host.active[serviceUnits[name]] {
			t.Fatalf("%s was not restarted", name)
		}
	}
	if host.active[serviceUnits["ssh"]] {
		t.Fatal("inactive service was unexpectedly started")
	}
}

func TestCatalogDryRunDoesNotMutateOrRestart(t *testing.T) {
	r, host, _ := catalogFixture(t)
	source := filepath.Join(t.TempDir(), "source")
	if err := catalogCopyInput(r.live, source); err != nil {
		t.Fatal(err)
	}
	catalogAddAlias(t, source)
	oldSig, _ := os.ReadFile(filepath.Join(r.live, "catalog.sig"))
	if err := r.apply(source, false, true); err != nil {
		t.Fatal(err)
	}
	newSig, _ := os.ReadFile(filepath.Join(r.live, "catalog.sig"))
	if !bytes.Equal(oldSig, newSig) || catalogHasAlias(t, r) {
		t.Fatal("dry run changed installed catalog")
	}
	for _, call := range host.calls {
		if !strings.HasPrefix(call, "visudo ") {
			t.Fatalf("dry run invoked service operation: %s", call)
		}
	}
	if _, err := os.Stat(filepath.Join(r.state, "pending")); !os.IsNotExist(err) {
		t.Fatal("dry run created recovery journal")
	}
}

func TestCatalogFailedStartOrPreflightAutomaticallyRestoresVerifiedCheckpoint(t *testing.T) {
	for _, failure := range []string{"stop", "start", "preflight", "visudo"} {
		t.Run(failure, func(t *testing.T) {
			r, host, _ := catalogFixture(t)
			source := filepath.Join(t.TempDir(), "source")
			if err := catalogCopyInput(r.live, source); err != nil {
				t.Fatal(err)
			}
			catalogAddAlias(t, source)
			failed := false
			host.fail = func(name string, args []string) bool {
				if failed {
					return false
				}
				hit := failure == "stop" && filepath.Base(name) == "systemctl" && args[0] == "stop" || failure == "start" && filepath.Base(name) == "systemctl" && args[0] == "start" || failure == "preflight" && strings.Contains(name, "preflight") || failure == "visudo" && strings.Contains(name, "visudo")
				if hit {
					failed = true
				}
				return hit
			}
			if err := r.apply(source, false, false); err == nil {
				t.Fatal("injected failure was ignored")
			}
			if catalogHasAlias(t, r) {
				t.Fatal("failed candidate remained active")
			}
			if _, err := os.Stat(filepath.Join(r.state, "pending")); !os.IsNotExist(err) {
				t.Fatal("successful recovery left pending journal")
			}
			for _, name := range []string{"identity", "logs", "gateway"} {
				if !host.active[serviceUnits[name]] {
					t.Fatalf("%s not restored", name)
				}
			}
		})
	}
}

func TestCatalogRejectsWrongKeyBadHashProfilesAndRemovedUserRolesBeforeStopping(t *testing.T) {
	for _, problem := range []string{"key", "hash", "profile", "role", "missing-key", "symlink"} {
		t.Run(problem, func(t *testing.T) {
			r, host, _ := catalogFixture(t)
			source := filepath.Join(t.TempDir(), "source")
			if err := catalogCopyInput(r.live, source); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.LoadConfig(source)
			if err != nil {
				t.Fatal(err)
			}
			switch problem {
			case "key":
				_, key, _ := ed25519.GenerateKey(rand.Reader)
				_ = os.WriteFile(r.key, integrity.EncodeBase64(key), 0600)
			case "missing-key":
				_ = os.Remove(r.key)
			case "hash":
				cfg.Executables[0].SHA256 = strings.Repeat("0", 64)
			case "profile":
				cfg.Profiles[0].MemoryLimitMiB++
			case "role":
				cfg.Roles[0].Name = "other"
				cfg.Policies[0].Role = "other"
			case "symlink":
				path := filepath.Join(source, "catalog.json")
				_ = os.Remove(path)
				_ = os.Symlink(filepath.Join(r.live, "catalog.json"), path)
			}
			if problem != "symlink" {
				if err = catalogWriteModel(source, cfg); err != nil {
					t.Fatal(err)
				}
			}
			if err = r.apply(source, false, false); err == nil {
				t.Fatal("unsafe candidate accepted")
			}
			for _, call := range host.calls {
				if strings.HasPrefix(call, "systemctl ") {
					t.Fatalf("validation failure mutated service state: %s", call)
				}
			}
			if _, err = os.Stat(filepath.Join(r.state, "pending")); !os.IsNotExist(err) {
				t.Fatal("validation failure created pending journal")
			}
		})
	}
}

func TestOfflineCatalogApplyRequiresExistingTrustAndNoLocalKey(t *testing.T) {
	r, _, key := catalogFixture(t)
	source := filepath.Join(t.TempDir(), "source")
	if err := catalogCopyInput(r.live, source); err != nil {
		t.Fatal(err)
	}
	catalogAddAlias(t, source)
	cfg, _ := config.LoadConfig(source)
	signature, _ := integrity.SignCatalog(cfg, key)
	if err := os.WriteFile(filepath.Join(source, "catalog.sig"), integrity.EncodeBase64(signature), 0644); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(r.key)
	if err := r.apply(source, true, false); err != nil {
		t.Fatal(err)
	}
	if !catalogHasAlias(t, r) {
		t.Fatal("offline-signed candidate not applied")
	}
}

func TestInterruptedCatalogTransactionCanBeRecoveredWithoutSigningKey(t *testing.T) {
	r, host, _ := catalogFixture(t)
	pending := filepath.Join(r.state, "pending")
	if err := r.snapshot(r.live, filepath.Join(pending, "baseline")); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(catalogJournal{Version: 1, Active: []string{"identity", "logs", "gateway"}})
	if err := os.WriteFile(filepath.Join(pending, "state.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.live, "catalog.json"), []byte("broken"), 0644); err != nil {
		t.Fatal(err)
	}
	for unit := range host.active {
		host.active[unit] = false
	}
	_ = os.Remove(r.key)
	if err := r.rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.verified(r.live); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatal("journal was not removed after recovery")
	}
}

func TestFailedCatalogRecoveryRetainsJournalForRetry(t *testing.T) {
	r, host, _ := catalogFixture(t)
	source := filepath.Join(t.TempDir(), "source")
	if err := catalogCopyInput(r.live, source); err != nil {
		t.Fatal(err)
	}
	catalogAddAlias(t, source)
	host.fail = func(name string, args []string) bool { return filepath.Base(name) == "systemctl" && args[0] == "start" }
	err := r.apply(source, false, false)
	if err == nil || !strings.Contains(err.Error(), "journal retained") {
		t.Fatalf("unexpected recovery result: %v", err)
	}
	if _, err = os.Stat(filepath.Join(r.state, "pending")); err != nil {
		t.Fatal("failed recovery discarded its journal")
	}
	host.fail = nil
	if err = r.rollback(); err != nil {
		t.Fatal(err)
	}
	if catalogHasAlias(t, r) {
		t.Fatal("recovery installed failed candidate")
	}
}

func TestCatalogNoBaselineCannotDestroyRunningDeployment(t *testing.T) {
	r, host, _ := catalogFixture(t)
	catalogAddAlias(t, r.live)
	_ = os.RemoveAll(filepath.Join(r.state, "current"))
	err := r.apply(r.live, false, false)
	if err == nil || !strings.Contains(err.Error(), "no verified rollback baseline") {
		t.Fatal(fmt.Sprintf("expected missing baseline refusal, got %v", err))
	}
	if len(host.calls) != 0 {
		t.Fatal("missing baseline mutated running services")
	}
}

func TestCatalogReapplyDoesNotRestartOrEraseRollbackHistory(t *testing.T) {
	r, host, _ := catalogFixture(t)
	catalogAddAlias(t, r.live)
	if err := r.apply(r.live, false, false); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.ReadFile(filepath.Join(r.state, "previous/catalog.sig"))
	host.calls = nil
	if err := r.apply(r.live, false, false); err != nil {
		t.Fatal(err)
	}
	for _, call := range host.calls {
		if !strings.HasPrefix(call, "visudo ") {
			t.Fatalf("unchanged catalog restarted services: %s", call)
		}
	}
	after, _ := os.ReadFile(filepath.Join(r.state, "previous/catalog.sig"))
	if !bytes.Equal(previous, after) {
		t.Fatal("no-op erased previous catalog")
	}
}
