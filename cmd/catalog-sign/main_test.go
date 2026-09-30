package main

import (
	"os"
	"path/filepath"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/integrity"
)

// TEST-INTEGRITY-015
func TestGenerateAndSignCatalogWithoutOverwritingKeys(t *testing.T) {
	dir := t.TempDir()
	privateKey := filepath.Join(dir, "catalog.key")
	publicKey := filepath.Join(dir, "catalog.pub")
	signature := filepath.Join(dir, "catalog.sig")
	if err := run(true, "", privateKey, publicKey, ""); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %o", info.Mode().Perm())
	}
	if err := run(true, "", privateKey, publicKey, ""); err == nil {
		t.Fatal("key generation overwrote existing files")
	}
	if err := run(false, "../../configs", privateKey, "", signature); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		t.Fatal(err)
	}
	if err := integrity.VerifyCatalogFiles(cfg, publicKey, signature); err != nil {
		t.Fatal(err)
	}
}
