//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"safeterm/internal/config"
	"safeterm/internal/siteconfig"
	"strings"
	"testing"
)

func TestSiteDefaultsCanBeCopiedAndEditedWithoutInternalCatalogFiles(t *testing.T) {
	cfg, err := config.LoadConfig("../../internal/defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ kind, name string }{{"role", "viewer"}, {"operation", "ping"}, {"profile", "diagnostics"}, {"executable", "uname"}, {"policy", "viewer"}} {
		definition, err := defaultDefinition(cfg, tc.kind, tc.name)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(definition)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 {
			t.Fatal("empty default")
		}
		if tc.kind == "operation" {
			if _, err := siteconfig.Parse[siteconfig.Operation]("operation.json", data); err != nil {
				t.Fatal(err)
			}
		}
		if tc.kind == "profile" {
			p, err := siteconfig.Parse[siteconfig.Profile]("profile.json", data)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = siteconfig.Prepare(cfg, siteconfig.Inputs{Profiles: []siteconfig.Named[siteconfig.Profile]{p}}); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func TestSiteLoaderIgnoresTemplateBeforeOpeningAndRejectsSourceLinks(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root file ownership boundary; executed by disposable-VM smoke")
	}
	root, err := os.MkdirTemp("/var/lib", "safeterm-site-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	if err = os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "roles.d")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/missing/template", filepath.Join(dir, "TEMPLATE.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active.json"), []byte(`{"name":"site-reader","description":"Read"}`), 0600); err != nil {
		t.Fatal(err)
	}
	in, err := loadSiteInputs(root)
	if err != nil || len(in.Roles) != 1 {
		t.Fatalf("load: %+v %v", in, err)
	}
	if err := os.Symlink(filepath.Join(dir, "active.json"), filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if _, err = loadSiteInputs(root); err == nil {
		t.Fatal("active symlink accepted")
	}
}
func TestManagedServiceNamesCannotEscapeReservedNamespace(t *testing.T) {
	for _, name := range []string{"site-../../ssh", "site-bad\n", "site-x.service", "site-", "unknown"} {
		if managedServiceUnit(name) != "" {
			t.Fatalf("accepted %q", name)
		}
	}
	if managedServiceUnit("site-example") != "safeterm-runner-site-example.service" {
		t.Fatal("incorrect unit mapping")
	}
	cfg, err := config.LoadConfig("../../internal/defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	profile := cfg.Profiles[0]
	profile.RunAsUser = "root"
	if _, err := siteconfig.RunnerUnit(profile); err == nil {
		t.Fatal("root identity accepted")
	}
	for _, p := range cfg.Profiles {
		unit, err := siteconfig.RunnerUnit(p)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(unit, "User=root") {
			t.Fatal("root unit")
		}
	}
}
