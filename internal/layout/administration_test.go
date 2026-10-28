package layout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAdministrationMigrationPreservesEveryInputAndMetadata(t *testing.T) {
	admin := t.TempDir()
	before := map[string]os.FileInfo{}
	for _, move := range FlatAdministration {
		path := filepath.Join(admin, move[0])
		if filepath.Ext(path) == ".d" || move[0] == "ssh" || move[0] == "console" {
			if err := os.Mkdir(path, 0750); err != nil {
				t.Fatal(err)
			}
			path = filepath.Join(path, "site.json")
		}
		if err := os.WriteFile(path, []byte("site bytes: "+move[0]), 0640); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		before[move[0]] = info
	}
	if CheckAdministration(admin) == nil {
		t.Fatal("flat runtime layout accepted")
	}
	if err := CheckAdministrationMigration(admin); err != nil {
		t.Fatal(err)
	}
	for _, move := range FlatAdministration {
		if _, err := os.Lstat(filepath.Join(admin, move[0])); err != nil {
			t.Fatal("preflight moved an input", err)
		}
	}
	if err := MigrateAdministration(admin); err != nil {
		t.Fatal(err)
	}
	if err := MigrateAdministration(admin); err != nil {
		t.Fatal("migration is not repeatable", err)
	}
	if err := CheckAdministration(admin); err != nil {
		t.Fatal(err)
	}
	for _, move := range FlatAdministration {
		if _, err := os.Lstat(filepath.Join(admin, move[0])); !os.IsNotExist(err) {
			t.Fatal("old input remains", move[0], err)
		}
		path := filepath.Join(admin, move[1])
		if before[move[0]].Name() == "site.json" {
			path = filepath.Join(path, "site.json")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "site bytes: "+move[0] {
			t.Fatal("input changed", move, err)
		}
		info, err := os.Stat(path)
		if err != nil || !os.SameFile(before[move[0]], info) || info.Mode() != before[move[0]].Mode() {
			t.Fatal("metadata/inode changed", move, err)
		}
	}
}

func TestAdministrationMigrationRefusesConflictsBeforeMovingAnything(t *testing.T) {
	for _, scenario := range []string{"conflict", "parent-symlink", "source-symlink", "legacy-catalog"} {
		t.Run(scenario, func(t *testing.T) {
			admin := t.TempDir()
			ssh := filepath.Join(admin, "ssh")
			if err := os.Mkdir(ssh, 0700); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "conflict":
				if err := os.WriteFile(filepath.Join(admin, "network.json"), []byte("old policy"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(admin, "network"), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(admin, "network/policy.json"), []byte("new policy"), 0600); err != nil {
					t.Fatal(err)
				}
			case "parent-symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(admin, "access")); err != nil {
					t.Fatal(err)
				}
			case "source-symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(admin, "roles.d")); err != nil {
					t.Fatal(err)
				}
			case "legacy-catalog":
				if err := os.WriteFile(filepath.Join(admin, "catalog.json"), []byte("old catalog"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := MigrateAdministration(admin); err == nil {
				t.Fatal("unsafe migration accepted")
			}
			if _, err := os.Stat(ssh); err != nil {
				t.Fatal("partial migration", err)
			}
		})
	}
}
