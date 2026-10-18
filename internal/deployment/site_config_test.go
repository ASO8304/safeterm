//go:build linux

package deployment

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/integrity"
	"safeterm/internal/siteconfig"
)

func TestSiteSudoersDoNotGrantAnotherProfileOperations(t *testing.T) {
	base, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := siteconfig.Prepare(base, siteconfig.Inputs{Profiles: []siteconfig.Named[siteconfig.Profile]{{Value: siteconfig.Profile{Name: "site-control", Base: config.ProfileServiceManager}}}})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Operations = append(cfg.Operations, config.OperationDefinition{Name: "site_status", RunnerProfile: "site-control", ExecutorKind: config.ExecutorSystemd, Systemd: &config.SystemdOperationDefinition{Unit: "site-only.service", Action: "status"}})
	lines, err := CatalogSudoers(cfg)
	if err != nil {
		t.Fatal(err)
	}
	siteFound, builtinFound := false, false
	for _, line := range lines {
		if strings.HasPrefix(line, "Cmnd_Alias SAFETERM_SITE_") {
			siteFound = true
			if !strings.Contains(line, "status site-only.service") || strings.Contains(line, "safeterm-ssh-broker.service") {
				t.Fatal("site profile inherited unrelated grants")
			}
		}
		if strings.HasPrefix(line, "Cmnd_Alias SAFETERM_SYSTEMD =") {
			builtinFound = true
			if strings.Contains(line, "site-only.service") {
				t.Fatal("builtin profile inherited site grant")
			}
		}
	}
	if !siteFound || !builtinFound {
		t.Fatal("missing scoped grants")
	}
}
func TestPackageStagingCanValidatePriorSignedIdentityRolesWithoutImportingDefinitions(t *testing.T) {
	bundle := buildTestBundle(t)
	root := filepath.Join(bundle, "etc/safeterm")
	cfg, err := config.LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "trust/catalog-ed25519.pub"), integrity.EncodeBase64(public), 0644); err != nil {
		t.Fatal(err)
	}
	signature, err := integrity.SignCatalog(cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "catalog.sig"), integrity.EncodeBase64(signature), 0644); err != nil {
		t.Fatal(err)
	}
	prior := filepath.Join(t.TempDir(), "prior")
	if err = PrepareConfigWithHashes(root, prior, false); err != nil {
		t.Fatal(err)
	}
	old, err := config.LoadConfig(prior)
	if err != nil {
		t.Fatal(err)
	}
	old.Roles = append(old.Roles, config.RoleDefinition{Name: "site-reader", Description: "Site"})
	writeJSON(t, filepath.Join(prior, "roles/roles.json"), old.Roles, 0644)
	signature, err = integrity.SignCatalog(old, key)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(prior, "catalog.sig"), integrity.EncodeBase64(signature), 0644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(root, "console/users.json"), map[string]any{"users": []any{map[string]any{"user_id": "site-user", "roles": []string{"site-reader"}, "disabled": true}}}, 0600)
	writeTestManifest(t, bundle)
	if _, err = Check(CheckOptions{Root: bundle}); err == nil {
		t.Fatal("ordinary staging silently accepted an unknown role")
	}
	if _, err = Check(CheckOptions{Root: bundle, PriorIdentityCatalog: prior}); err != nil {
		t.Fatal(err)
	}
	after, err := config.LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Roles) != len(cfg.Roles) {
		t.Fatal("prior role definition imported into incoming model")
	}
	if err = os.WriteFile(filepath.Join(prior, "catalog.sig"), []byte("invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = Check(CheckOptions{Root: bundle, PriorIdentityCatalog: prior}); err == nil {
		t.Fatal("unverified prior identity roles accepted")
	}
}
