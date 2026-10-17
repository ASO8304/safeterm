//go:build linux

package deployment

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/integrity"
)

func TestPortableCatalogPreservesCustomizationAndExcludesHostSecrets(t *testing.T) {
	bundle := buildTestBundle(t)
	source := filepath.Join(bundle, "etc/safeterm")
	cfg, err := config.LoadConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Operations[0].Description = "Customer log aliases and operations"
	writeJSON(t, filepath.Join(source, "operations/operations.json"), cfg.Operations, 0644)
	output := filepath.Join(t.TempDir(), "portable")
	if err := PrepareConfigWithHashes(source, output, false); err != nil {
		t.Fatal(err)
	}
	got, err := config.LoadConfig(output)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatal("export changed customized catalog semantics or approved hashes")
	}
	for _, path := range []string{"ssh", "console", "config-manager", "catalog.sig", "trust/catalog-ed25519.pub"} {
		if _, err := os.Lstat(filepath.Join(output, path)); !os.IsNotExist(err) {
			t.Fatalf("export leaked host data: %s", path)
		}
	}
}

func TestUpgradePreservesSignatureAndApprovedHashes(t *testing.T) {
	bundle := buildTestBundle(t)
	source := filepath.Join(bundle, "etc/safeterm")
	output := filepath.Join(t.TempDir(), "upgrade")
	if err := PrepareConfigWithHashes(source, output, false); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := integrity.VerifyCatalogFiles(cfg, filepath.Join(source, "trust/catalog-ed25519.pub"), filepath.Join(source, "catalog.sig")); err != nil {
		t.Fatal("normalization invalidated approved signature:", err)
	}
	// An ordinary update must never silently approve a changed executable.
	cfg.Executables[0].SHA256 = strings.Repeat("a", 64)
	writeJSON(t, filepath.Join(source, "executables/executables.json"), cfg.Executables, 0644)
	kept := filepath.Join(t.TempDir(), "kept")
	if err := PrepareConfigWithHashes(source, kept, false); err != nil {
		t.Fatal(err)
	}
	got, err := config.LoadConfig(kept)
	if err != nil {
		t.Fatal(err)
	}
	if got.Executables[0].SHA256 != cfg.Executables[0].SHA256 {
		t.Fatal("upgrade approved new executable content")
	}
	if _, err := integrity.NewExecutor(got, got.Profiles[0].Name, verificationExecutor{}); err == nil {
		t.Fatal("unapproved executable passed verification")
	}
}

func TestInstallConfigurationPreservesSiteFilesAndRemovesStaleCatalogFragments(t *testing.T) {
	bundle := buildTestBundle(t)
	source := filepath.Join(bundle, "etc/safeterm")
	for _, name := range []string{"README.md", "README.txt"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte("reference"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(t.TempDir(), "installed")
	if err := InstallConfiguration(source, target); err != nil {
		t.Fatal(err)
	}
	registryInfo, err := os.Stat(filepath.Join(target, "config-manager/resources.json"))
	if err != nil || registryInfo.Mode().Perm() != 0644 {
		t.Fatalf("fresh registry must be readable by services: info=%v err=%v", registryInfo, err)
	}
	secrets := []string{"ssh/users.json", "ssh/ssh_host_ed25519_key", "console/users.json", "config-manager/resources.json", "site-private.ini"}
	for _, name := range secrets {
		if err := os.WriteFile(filepath.Join(target, name), []byte("customer-specific "+name), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(target, name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	stale := filepath.Join(target, "operations/old-fragment.json")
	if err := os.WriteFile(stale, []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := InstallConfiguration(source, target); err != nil {
		t.Fatal(err)
	}
	for _, name := range secrets {
		path := filepath.Join(target, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "customer-specific "+name || info.Mode().Perm() != 0600 {
			t.Fatalf("upgrade changed site file contents or permissions: %s", name)
		}
	}
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Fatal("stale operation fragment would duplicate preserved operations")
	}
	if err := os.RemoveAll(filepath.Join(target, "operations")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(target, "operations")); err != nil {
		t.Fatal(err)
	}
	if err := InstallConfiguration(source, target); err == nil {
		t.Fatal("symlinked catalog directory accepted")
	}
}

func TestUpgradeStagesCustomResourceWithoutChangingApplicationMetadata(t *testing.T) {
	bundle := buildTestBundle(t)
	target := filepath.Join(t.TempDir(), "customer.ini")
	if err := os.WriteFile(target, []byte("[service]\nport=9012\n"), 0600); err != nil {
		t.Fatal(err)
	}
	registry := configmanager.Registry{Version: "site", Resources: []configmanager.Resource{{ID: "site-options", Description: "Site", Profile: "application", Path: target, Format: configmanager.FormatINI, Access: configmanager.AccessReadOnly, Fields: []configmanager.FieldSchema{{Section: "service", Key: "port", Type: configmanager.FieldInteger, Required: true}}, Permissions: configmanager.Permissions{ReadRoles: []string{"viewer"}}}}}
	writeJSON(t, filepath.Join(bundle, "etc/safeterm/config-manager/resources.json"), registry, 0644)
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := StageManagedUpgrade(bundle); err != nil {
		t.Fatal(err)
	}
	files, err := ManagedFiles(bundle)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(files[0].Source)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[service]\nport=9012\n" || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.ModTime() != after.ModTime() {
		t.Fatal("staging changed live application state")
	}
}

func TestUpgradeRejectsIncompatibleProfileBeforePreparation(t *testing.T) {
	source, _ := writeSourceCatalog(t)
	defaults := filepath.Join(t.TempDir(), "defaults")
	if err := PrepareConfigWithHashes(source, defaults, false); err != nil {
		t.Fatal(err)
	}
	if err := CheckProfileCompatibility(source, defaults); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profiles[0].MemoryLimitMiB++
	writeJSON(t, filepath.Join(source, "profiles/profiles.json"), cfg.Profiles, 0644)
	if err := CheckProfileCompatibility(source, defaults); err == nil {
		t.Fatal("incompatible sandbox boundaries accepted")
	}
}
