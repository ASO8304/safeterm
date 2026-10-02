//go:build linux

package deployment

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/integrity"
)

// TEST-DEPLOY-001: REQ-015 AC-1 and AC-2.
func TestPrepareConfigRefreshesDigestsWithoutPublishingSigningMaterial(t *testing.T) {
	source, executable := writeSourceCatalog(t)
	output := filepath.Join(t.TempDir(), "prepared")
	if err := PrepareConfig(source, output); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(output)
	if err != nil {
		t.Fatal(err)
	}
	want, err := executableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Executables) != 1 || cfg.Executables[0].SHA256 != want {
		t.Fatalf("prepared executables=%+v want digest=%s", cfg.Executables, want)
	}
	if _, err := os.Stat(filepath.Join(output, "catalog.sig")); !os.IsNotExist(err) {
		t.Fatal("unsigned preparation unexpectedly produced a signature")
	}
	if err := PrepareConfig(source, output); err == nil {
		t.Fatal("existing output directory was overwritten")
	}
}

// TEST-DEPLOY-002: REQ-015 AC-3 and AC-4.
func TestBundlePreflightAcceptsCompleteBundleAndRejectsTampering(t *testing.T) {
	bundle := buildTestBundle(t)
	report, err := Check(CheckOptions{Root: bundle})
	if err != nil {
		t.Fatal(err)
	}
	if report.CatalogVersion != "m15-test" || report.Profiles != 1 || report.Operations != 1 || report.Resources != 1 || report.Units != len(SystemdUnits) {
		t.Fatalf("report=%+v", report)
	}
	signature := filepath.Join(bundle, "etc", "safeterm", "catalog.sig")
	if err := os.WriteFile(signature, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(CheckOptions{Root: bundle}); err == nil {
		t.Fatal("tampered signature passed preflight")
	}
}

func buildTestBundle(t *testing.T) string {
	t.Helper()
	source, _ := writeSourceCatalog(t)
	bundle := t.TempDir()
	configDir := filepath.Join(bundle, "etc", "safeterm")
	if err := PrepareConfig(source, configDir); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(configDir)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := integrity.SignCatalog(cfg, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "trust", "catalog-ed25519.pub"), integrity.EncodeBase64(publicKey), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "catalog.sig"), integrity.EncodeBase64(signature), 0o644); err != nil {
		t.Fatal(err)
	}
	sshDir := filepath.Join(configDir, "ssh")
	if err := os.Mkdir(sshDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, hostPrivate, _ := ed25519.GenerateKey(rand.Reader)
	hostBlock, err := ssh.MarshalPrivateKey(hostPrivate, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "ssh_host_ed25519_key"), pem.EncodeToMemory(hostBlock), 0o600); err != nil {
		t.Fatal(err)
	}
	clientPublic, _, _ := ed25519.GenerateKey(rand.Reader)
	sshPublic, err := ssh.NewPublicKey(clientPublic)
	if err != nil {
		t.Fatal(err)
	}
	identities := map[string]any{"users": []any{map[string]any{"user_id": "alice", "roles": []string{"viewer"}, "authorized_keys": []string{string(ssh.MarshalAuthorizedKey(sshPublic))}}}}
	writeJSON(t, filepath.Join(sshDir, "users.json"), identities, 0o600)
	registry := configmanager.Registry{Version: "m15", Resources: []configmanager.Resource{{ID: "demo-options", Description: "Demo", Profile: "application", Path: "/etc/safeterm-managed/application/options.json", Format: configmanager.FormatJSON, Access: configmanager.AccessReadOnly, Fields: []configmanager.FieldSchema{{Key: "enabled", Type: configmanager.FieldBoolean, Required: true}}, Permissions: configmanager.Permissions{ReadRoles: []string{"viewer"}}}}}
	if err := os.Mkdir(filepath.Join(configDir, "config-manager"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(configDir, "config-manager", "resources.json"), registry, 0o644)
	if err := os.MkdirAll(filepath.Join(bundle, "managed", "application"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(bundle, "managed", "application", "options.json"), map[string]bool{"enabled": true}, 0o600)
	executable, _ := os.Executable()
	if err := os.Mkdir(filepath.Join(bundle, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range RuntimeBinaries {
		copyFile(t, executable, filepath.Join(bundle, "bin", name), 0o555)
	}
	if err := os.Mkdir(filepath.Join(bundle, "systemd"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range SystemdUnits {
		data := []byte("[Service]\nNoNewPrivileges=yes\nMemoryMax=64M\nTasksMax=16\n")
		if err := os.WriteFile(filepath.Join(bundle, "systemd", name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeTestManifest(t, bundle)
	return bundle
}

func writeTestManifest(t *testing.T, root string) {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest, err := fileDigest(path)
		if err != nil {
			return err
		}
		lines = append(lines, digest+"  "+filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	if err := os.WriteFile(filepath.Join(root, manifestName), []byte(strings.Join(lines, "\n")+"\n"), 0o444); err != nil {
		t.Fatal(err)
	}
}

func writeSourceCatalog(t *testing.T) (string, string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	for _, directory := range []string{"operations", "executables", "profiles", "roles", "policies"} {
		if err := os.Mkdir(filepath.Join(source, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(t, filepath.Join(source, "catalog.json"), map[string]string{"version": "m15-test"}, 0o644)
	writeJSON(t, filepath.Join(source, "operations", "operations.json"), []config.OperationDefinition{{Name: "system_info", Description: "Demo", RunnerProfile: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, ExecutableRef: "demo", Parameters: []config.ParameterDefinition{}}}, 0o644)
	writeJSON(t, filepath.Join(source, "executables", "executables.json"), []config.ExecutableDefinition{{ID: "demo", Path: executable, SHA256: strings.Repeat("0", 64), AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics}}}, 0o644)
	writeJSON(t, filepath.Join(source, "profiles", "profiles.json"), []config.ProfileDefinition{{Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/safeterm-demo.sock", RunAsUser: "safeterm-diagnostics", MemoryLimitMiB: 64, ProcessLimit: 8}}, 0o644)
	writeJSON(t, filepath.Join(source, "roles", "roles.json"), []config.RoleDefinition{{Name: "viewer", Description: "Viewer"}}, 0o644)
	writeJSON(t, filepath.Join(source, "policies", "policies.json"), []config.PolicyDefinition{{Role: "viewer", Operations: []string{"system_info"}}}, 0o644)
	return source, executable
}

func writeJSON(t *testing.T, path string, value any, mode os.FileMode) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), mode); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, source, target string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, mode); err != nil {
		t.Fatal(err)
	}
}
