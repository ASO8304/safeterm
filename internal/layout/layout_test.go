package layout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledPathsHaveNoLegacyFallback(t *testing.T) {
	if Catalog(Admin) != Generated || Authority(Generated) != Admin || PublicKey(Generated) != Trust {
		t.Fatal("installed paths do not separate authority and generated data")
	}
	for _, name := range []string{"config-manager", "resources.json", "catalog.json", "operations", "policies"} {
		dir := t.TempDir()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("must remain untouched"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := RejectLegacy(dir); err == nil {
			t.Fatalf("accepted %s", name)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "must remain untouched" {
			t.Fatal("refusal modified legacy data")
		}
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "resources.d"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := RejectLegacy(dir); err != nil {
		t.Fatal(err)
	}
}
