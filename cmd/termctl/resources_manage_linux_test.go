//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/integrity"
	"safeterm/internal/resources"
)

func resourceFixture(t *testing.T) (resourceRuntime, *catalogFakeHost) {
	t.Helper()
	c, host, _ := catalogFixture(t)
	root := filepath.Dir(c.live)
	c.extraFiles = map[string]string{"registry.json": filepath.Join(c.live, "resources.json"), "writer.conf": filepath.Join(root, "writer.conf"), "manifest.json": filepath.Join(c.live, ".resources-manifest.json")}
	for name, data := range map[string][]byte{"writer.conf": []byte("old sandbox\n")} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.snapshot(c.live, filepath.Join(root, "builtins")); err != nil {
		t.Fatal(err)
	}
	if err := c.saveCheckpoint("current", c.live); err != nil {
		t.Fatal(err)
	}
	return resourceRuntime{catalog: c, defaults: filepath.Join(root, "builtins"), plan: func(reg *configmanager.Registry, _ bool) ([]byte, error) { return json.Marshal(reg) }, logs: func(resources.Log) error { return nil }}, host
}
func serviceItem(t *testing.T) resources.Named {
	t.Helper()
	item, err := resources.Parse("resources.d/example.json", []byte(`{"id":"example","description":"Example","service":{"unit":"example.service","actions":{"status":{"roles":["viewer"]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return item
}
func TestResourceLoaderNeverOpensTemplateOrInactiveFiles(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{"TEMPLATE.json": "INVALID TEMPLATE MUST BE IGNORED", "notes.txt": "not JSON", "example.json.disabled": "ignored", "active.json": `{"id":"logs","description":"Logs","logs":[{"id":"main","path":"/var/log/example.log","roles":["viewer"]}]}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	opened := []string{}
	items, err := loadResourceDefinitions(dir, func(path string) ([]byte, error) {
		opened = append(opened, filepath.Base(path))
		return os.ReadFile(path)
	})
	if err != nil || len(items) != 1 || strings.Join(opened, ",") != "active.json" {
		t.Fatalf("items=%v opened=%v err=%v", items, opened, err)
	}
	if err := os.Symlink(filepath.Join(dir, "active.json"), filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := loadResourceDefinitions(dir, catalogRead); err == nil {
		t.Fatal("active symlink accepted")
	}
}
func TestResourceCheckNeedsNoSigningKeyAndApplyIsIdempotent(t *testing.T) {
	r, host := resourceFixture(t)
	item := serviceItem(t)
	oldKey := r.catalog.key
	r.catalog.key = "/no/key"
	before, _ := catalogRead(filepath.Join(r.catalog.live, "catalog.sig"))
	oldCalls := len(host.calls)
	if err := r.apply([]resources.Named{item}, true, ""); err != nil {
		t.Fatal(err)
	}
	after, _ := catalogRead(filepath.Join(r.catalog.live, "catalog.sig"))
	if !bytes.Equal(before, after) {
		t.Fatal("check mutated live state")
	}
	for _, call := range host.calls[oldCalls:] {
		if strings.Contains(call, "systemctl stop") || strings.Contains(call, "systemctl start") {
			t.Fatal("check restarted services")
		}
	}
	r.catalog.key = oldKey
	if err := r.apply([]resources.Named{item}, false, ""); err != nil {
		t.Fatal(err)
	}
	cfg, err := r.catalog.verified(r.catalog.live)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, op := range cfg.Operations {
		found = found || op.Name == resources.OperationID("example", "service", "status")
	}
	if !found {
		t.Fatal("missing operation")
	}
	oldCalls = len(host.calls)
	if err := r.apply([]resources.Named{item}, false, ""); err != nil {
		t.Fatal(err)
	}
	for _, call := range host.calls[oldCalls:] {
		if strings.HasPrefix(call, "systemctl stop") {
			t.Fatal("unchanged definitions restarted services")
		}
	}
	if err := r.apply(nil, false, ""); err != nil {
		t.Fatal(err)
	}
	cfg, _ = r.catalog.verified(r.catalog.live)
	for _, op := range cfg.Operations {
		if op.Name == resources.OperationID("example", "service", "status") {
			t.Fatal("removed source still exposed")
		}
	}
}
func TestUnifiedActivationFailureRestoresCatalogRegistryManifestAndSandbox(t *testing.T) {
	for _, stage := range []string{"publish", "preflight", "start"} {
		t.Run(stage, func(t *testing.T) {
			r, host := resourceFixture(t)
			item := combinedItem(t)
			before := map[string][]byte{}
			for name, path := range r.catalog.extraFiles {
				before[name], _ = catalogRead(path)
			}
			signature, _ := catalogRead(filepath.Join(r.catalog.live, "catalog.sig"))
			active := map[string]bool{}
			for k, v := range host.active {
				active[k] = v
			}
			failed := false
			host.fail = func(name string, args []string) bool {
				if failed {
					return false
				}
				fail := stage == "preflight" && strings.Contains(name, "preflight") || stage == "start" && filepath.Base(name) == "systemctl" && args[0] == "start" || stage == "publish" && filepath.Base(name) == "systemctl" && args[0] == "daemon-reload"
				if fail {
					failed = true
				}
				return fail
			}
			if err := r.apply([]resources.Named{item}, false, ""); err == nil {
				t.Fatal("injected failure accepted")
			}
			after, _ := catalogRead(filepath.Join(r.catalog.live, "catalog.sig"))
			if !bytes.Equal(signature, after) {
				t.Fatal("catalog not restored")
			}
			for name, path := range r.catalog.extraFiles {
				data, err := catalogRead(path)
				if name == "manifest.json" && len(before[name]) == 0 {
					var m resources.Manifest
					if err == nil {
						_ = json.Unmarshal(data, &m)
						if len(m.Operations) > 0 {
							t.Fatal("manifest not restored")
						}
					}
					continue
				}
				if err != nil || !bytes.Equal(before[name], data) {
					t.Fatalf("%s not restored: %s (%v)", name, data, err)
				}
			}
			if !reflect.DeepEqual(host.active, active) {
				t.Fatalf("active=%v expected=%v", host.active, active)
			}
			if _, err := os.Stat(filepath.Join(r.catalog.state, "pending")); !os.IsNotExist(err) {
				t.Fatal("successful recovery retained journal")
			}
		})
	}
}
func TestUnifiedRecoveryRetainsJournalUntilRecoverySucceeds(t *testing.T) {
	r, host := resourceFixture(t)
	host.fail = func(name string, args []string) bool { return filepath.Base(name) == "systemctl" && args[0] == "start" }
	if err := r.apply([]resources.Named{combinedItem(t)}, false, ""); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(filepath.Join(r.catalog.state, "pending")); err != nil {
		t.Fatal("lost recovery journal")
	}
	host.fail = nil
	if err := r.catalog.recoverPending(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.catalog.state, "pending")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("journal not cleared")
	}
}

func TestOfflineResourceExportMatchesSignedCandidate(t *testing.T) {
	r, _ := resourceFixture(t)
	item := serviceItem(t)
	r.export = filepath.Join(t.TempDir(), "unsigned")
	r.exportCheck = func(string) error { return nil }
	before, _ := catalogRead(filepath.Join(r.catalog.live, "catalog.sig"))
	if err := r.apply([]resources.Named{item}, false, ""); err != nil {
		t.Fatal(err)
	}
	after, _ := catalogRead(filepath.Join(r.catalog.live, "catalog.sig"))
	if !bytes.Equal(before, after) {
		t.Fatal("export activated candidate")
	}
	cfg, err := config.LoadConfig(r.export)
	if err != nil {
		t.Fatal(err)
	}
	key, err := integrity.ReadPrivateKey(r.catalog.key)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	sig, err := integrity.SignCatalog(cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(r.export, "catalog.sig"), integrity.EncodeBase64(sig), 0644); err != nil {
		t.Fatal(err)
	}
	signed := r.export
	r.export = ""
	if err = r.apply([]resources.Named{item}, false, signed); err != nil {
		t.Fatal(err)
	}
	item.Definition.Service.Unit = "changed.service"
	if err = r.apply([]resources.Named{item}, false, signed); err == nil {
		t.Fatal("mismatched offline-signed candidate accepted")
	}
}

func TestInvalidDefinitionsOrFileAccessNeverStopServices(t *testing.T) {
	r, host := resourceFixture(t)
	item := serviceItem(t)
	item.Definition.Logs = []resources.Log{{ID: "main", Path: "/var/log/product/main.log", Roles: []string{"viewer"}}}
	// Fixture lacks tail/profile; a valid service-only definition is used to test
	// injected filesystem validation before any signing/activation.
	item.Definition.Logs = nil
	r.plan = func(*configmanager.Registry, bool) ([]byte, error) {
		return nil, errors.New("unsafe application namespace")
	}
	oldCalls := len(host.calls)
	if err := r.apply([]resources.Named{item}, false, ""); err == nil {
		t.Fatal("invalid file plan accepted")
	}
	if len(host.calls) != oldCalls {
		t.Fatal("validation failure reached activation")
	}
}

func combinedItem(t *testing.T) resources.Named {
	item := serviceItem(t)
	item.Definition.Config = &resources.Configuration{Path: "/etc/example/options.ini", Format: configmanager.FormatINI, Permissions: resources.Permissions{Read: []string{"viewer"}, Edit: []string{"viewer"}, Rollback: []string{"viewer"}}, Fields: []configmanager.FieldSchema{{Section: "Server", Key: "Port", Type: configmanager.FieldInteger}}}
	return item
}

func TestCompletedResourceRollbackSwapsAllAssociatedMetadata(t *testing.T) {
	r, _ := resourceFixture(t)
	if err := r.apply([]resources.Named{combinedItem(t)}, false, ""); err != nil {
		t.Fatal(err)
	}
	applied := map[string][]byte{}
	for name, path := range r.catalog.extraFiles {
		applied[name], _ = catalogRead(path)
	}
	if err := r.catalog.rollback(); err != nil {
		t.Fatal(err)
	}
	registryData, err := catalogRead(r.catalog.extraFiles["registry.json"])
	if err != nil {
		t.Fatal(err)
	}
	reg, err := configmanager.ParseRegistry(registryData)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range reg.Resources {
		if resource.ID == "example" {
			t.Fatal("rollback retained generated config")
		}
	}
	writer, _ := catalogRead(r.catalog.extraFiles["writer.conf"])
	if string(writer) != "old sandbox\n" {
		t.Fatal("writer sandbox not restored")
	}

	if err := r.catalog.rollback(); err != nil {
		t.Fatal(err)
	}
	for name, path := range r.catalog.extraFiles {
		data, err := catalogRead(path)
		if err != nil || !bytes.Equal(data, applied[name]) {
			t.Fatalf("%s did not swap on rollback: %v", name, err)
		}
	}
}

func TestResourceRebuildDoesNotImportActiveCatalogOrRegistry(t *testing.T) {
	r, _ := resourceFixture(t)
	catalogAddAlias(t, r.catalog.live)
	cfg, err := config.LoadConfig(r.catalog.live)
	if err != nil {
		t.Fatal(err)
	}
	key, err := integrity.ReadPrivateKey(r.catalog.key)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	sig, err := integrity.SignCatalog(cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(r.catalog.live, "catalog.sig"), integrity.EncodeBase64(sig), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(r.catalog.extraFiles["registry.json"], []byte("not a source registry"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = r.apply(nil, false, ""); err != nil {
		t.Fatal(err)
	}
	if catalogHasAlias(t, r.catalog) {
		t.Fatal("imported active-only alias into rebuilt catalog")
	}
	data, err := os.ReadFile(r.catalog.extraFiles["registry.json"])
	if err != nil {
		t.Fatal(err)
	}
	reg, err := configmanager.ParseRegistry(data)
	if err != nil || len(reg.Resources) != 0 {
		t.Fatalf("generated registry not rebuilt: %v", err)
	}
}

func TestIncompleteResourceSnapshotCannotFallBackToLive(t *testing.T) {
	r, _ := resourceFixture(t)
	incomplete := t.TempDir()
	if err := r.catalog.snapshotExtras(incomplete, t.TempDir()); err == nil {
		t.Fatal("missing checkpoint extras silently replaced from live")
	}
	before, err := os.ReadFile(r.catalog.extraFiles["registry.json"])
	if err != nil {
		t.Fatal(err)
	}
	if err := r.catalog.publishExtras(incomplete); err == nil {
		t.Fatal("incomplete resource checkpoint accepted")
	}
	after, _ := os.ReadFile(r.catalog.extraFiles["registry.json"])
	if !bytes.Equal(before, after) {
		t.Fatal("failed restore mutated live registry")
	}
}

func TestCatalogLogProbeAllowsOnlyFixedUnprivilegedReadCheck(t *testing.T) {
	valid := []string{"-n", "-u", "safeterm-log-reader", "-g", "safeterm-gateway", "--", "/usr/bin/test", "-r", "/var/log/product/service.log"}
	if err := validateCatalogCommand("/usr/bin/sudo", valid); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		index int
		value string
	}{{2, "root"}, {4, "root"}, {6, "/bin/sh"}, {7, "-w"}, {8, "/root/private.log"}, {8, "relative.log"}} {
		args := append([]string(nil), valid...)
		args[tc.index] = tc.value
		if err := validateCatalogCommand("/usr/bin/sudo", args); err == nil {
			t.Errorf("accepted unsafe probe %v", args)
		}
	}
	for _, args := range [][]string{nil, valid[:8], append(append([]string(nil), valid...), "extra")} {
		if err := validateCatalogCommand("/usr/bin/sudo", args); err == nil {
			t.Fatal("accepted malformed probe")
		}
	}
	if _, err := catalogRun("/usr/bin/sudo", "/bin/sh"); err == nil || !strings.Contains(err.Error(), "unsupported catalog administration arguments") {
		t.Fatalf("unsafe invocation was not rejected: %v", err)
	}
}
