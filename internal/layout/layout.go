// Package layout defines the single supported installed configuration layout.
package layout

import (
	"fmt"
	"os"
	"path/filepath"
)

const Admin = "/etc/safeterm"
const Generated = "/var/lib/safeterm/generated"
const Registry = Generated + "/resources.json"
const Defaults = "/usr/share/safeterm/defaults"
const Trust = Admin + "/trust/catalog-ed25519.pub"
const AuditStore = "/var/lib/safeterm/audit/audit.jsonl"

// Catalog and Authority permit explicit isolated staging/test roots. Installed
// processes use Generated and never probe the old administrator catalog paths.
func Catalog(admin string) string {
	if filepath.Clean(admin) == Admin {
		return Generated
	}
	return admin
}
func Authority(catalog string) string {
	if filepath.Clean(catalog) == Generated {
		return Admin
	}
	return catalog
}
func PublicKey(catalog string) string {
	return filepath.Join(Authority(catalog), "trust/catalog-ed25519.pub")
}

// RejectLegacy is a refusal, not a migration or fallback. No files are deleted.
func RejectLegacy(admin string) error {
	for _, name := range []string{"config-manager", "operations", "policies", "profiles", "executables", "roles", "resources.json", "catalog.json", "catalog.sig", ".resources-manifest.json"} {
		if _, err := os.Lstat(filepath.Join(admin, name)); err == nil {
			return fmt.Errorf("unsupported legacy configuration %s: use a clean resources.d installation; no automatic import or deletion is performed", filepath.Join(admin, name))
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
