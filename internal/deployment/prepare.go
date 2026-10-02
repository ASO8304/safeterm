// Package deployment provides fail-closed preparation and verification for
// SafeTerm deployment bundles. Runtime services never consume a bundle
// directly; the installer copies a verified bundle into root-owned paths.
package deployment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"safeterm/internal/config"
)

// PrepareConfig creates a normalized unsigned catalog whose executable
// digests match the target host. The result must be signed offline before it
// can be accepted by CheckBundle or any runtime service.
func PrepareConfig(source, output string) error {
	if source == "" || output == "" {
		return fmt.Errorf("deployment: source and output directories are required")
	}
	if _, err := os.Lstat(output); err == nil {
		return fmt.Errorf("deployment: output directory already exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("deployment: inspect output directory: %w", err)
	}
	cfg, err := config.LoadConfig(source)
	if err != nil {
		return err
	}
	for index := range cfg.Executables {
		digest, err := executableDigest(cfg.Executables[index].Path)
		if err != nil {
			return fmt.Errorf("deployment: hash executable %q: %w", cfg.Executables[index].ID, err)
		}
		cfg.Executables[index].SHA256 = digest
	}
	parent, err := filepath.Abs(filepath.Dir(output))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".safeterm-config-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	for _, directory := range []string{"operations", "executables", "profiles", "roles", "policies", "trust"} {
		if err := os.Mkdir(filepath.Join(staging, directory), 0o755); err != nil {
			return err
		}
	}
	files := []struct {
		path  string
		value any
	}{
		{"catalog.json", struct {
			Version string `json:"version"`
		}{cfg.Version}},
		{"operations/operations.json", cfg.Operations},
		{"executables/executables.json", cfg.Executables},
		{"profiles/profiles.json", cfg.Profiles},
		{"roles/roles.json", cfg.Roles},
		{"policies/policies.json", cfg.Policies},
	}
	for _, file := range files {
		data, err := json.MarshalIndent(file.value, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if err := os.WriteFile(filepath.Join(staging, filepath.FromSlash(file.path)), data, 0o644); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, output); err != nil {
		return fmt.Errorf("deployment: publish prepared catalog: %w", err)
	}
	return nil
}

func executableDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 {
		return "", fmt.Errorf("target must be a non-writable regular executable")
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
