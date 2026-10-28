//go:build linux

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerStagesExistingCredentialsAcrossLayoutUpgrade(t *testing.T) {
	support, err := filepath.Abs("../../deploy/package/install-support.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"fresh", "flat", "grouped", "conflict"} {
		t.Run(scenario, func(t *testing.T) {
			admin := t.TempDir()
			if scenario == "flat" || scenario == "conflict" {
				if err := os.Mkdir(filepath.Join(admin, "ssh"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "grouped" || scenario == "conflict" {
				if err := os.MkdirAll(filepath.Join(admin, "access/ssh"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("sh", "-c", `. "$SUPPORT"; safeterm_admin_source "$ADMIN" access/ssh ssh`)
			cmd.Env = append(os.Environ(), "SUPPORT="+support, "ADMIN="+admin)
			out, err := cmd.CombinedOutput()
			if scenario == "conflict" {
				if err == nil || !strings.Contains(string(out), "conflict") {
					t.Fatalf("conflict accepted: %s %v", out, err)
				}
				return
			}
			want := filepath.Join(admin, "access/ssh")
			if scenario == "flat" {
				want = filepath.Join(admin, "ssh")
			}
			if err != nil || strings.TrimSpace(string(out)) != want {
				t.Fatalf("credential source: %s %v", out, err)
			}
		})
	}
}

func TestInstallerCleansOnlyEmptyLegacyDirectories(t *testing.T) {
	support, err := filepath.Abs("../../deploy/package/install-support.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"absent", "empty", "profiles", "data", "hidden", "unknown", "symlink", "profile-symlink", "file"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			legacy := filepath.Join(root, "safeterm-managed")
			if scenario != "absent" && scenario != "symlink" {
				for _, profile := range []string{"application", "database", "network"} {
					if err := os.MkdirAll(filepath.Join(legacy, profile), 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			outside := filepath.Join(root, "outside")
			if err := os.Mkdir(outside, 0700); err != nil {
				t.Fatal(err)
			}
			write := func(path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("preserve me"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(outside, "data"))
			switch scenario {
			case "empty":
				for _, profile := range []string{"application", "database", "network"} {
					if err := os.Remove(filepath.Join(legacy, profile)); err != nil {
						t.Fatal(err)
					}
				}
			case "data":
				write(filepath.Join(legacy, "network", "options.ini"))
			case "hidden":
				write(filepath.Join(legacy, ".site-data"))
			case "unknown":
				if err := os.Mkdir(filepath.Join(legacy, "custom"), 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(outside, legacy); err != nil {
					t.Fatal(err)
				}
			case "profile-symlink", "file":
				path := filepath.Join(legacy, "network")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if scenario == "file" {
					write(path)
				} else if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("sh", "-c", `set -eu; . "$SUPPORT"; safeterm_cleanup_legacy_managed "$LEGACY"; safeterm_cleanup_legacy_managed "$LEGACY"`)
			cmd.Env = append(os.Environ(), "SUPPORT="+support, "LEGACY="+legacy)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("cleanup: %s: %v", out, err)
			}
			removed := scenario == "absent" || scenario == "empty" || scenario == "profiles"
			_, statErr := os.Lstat(legacy)
			if removed != os.IsNotExist(statErr) {
				t.Fatalf("legacy state: %v; output: %s", statErr, out)
			}
			if !removed && !strings.Contains(string(out), "Retained") {
				t.Fatalf("missing retention explanation: %s", out)
			}
			if !removed && scenario != "symlink" {
				if _, err := os.Stat(filepath.Join(legacy, "application")); err != nil {
					t.Fatal("partial cleanup", err)
				}
			}
			if data, err := os.ReadFile(filepath.Join(outside, "data")); err != nil || string(data) != "preserve me" {
				t.Fatal("external data changed", err)
			}
		})
	}
}
