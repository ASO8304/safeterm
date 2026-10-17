package deployment

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
)

// CheckProfileCompatibility prevents a catalog from requesting isolation
// boundaries that the package's service accounts and systemd units do not provide.
func CheckProfileCompatibility(source, defaults string) error {
	load := func(path string) ([]config.ProfileDefinition, error) {
		cfg, err := config.LoadConfig(path)
		if err != nil {
			return nil, err
		}
		for i := range cfg.Profiles {
			sort.Strings(cfg.Profiles[i].Capabilities)
		}
		sort.Slice(cfg.Profiles, func(i, j int) bool { return cfg.Profiles[i].Name < cfg.Profiles[j].Name })
		return cfg.Profiles, nil
	}
	a, err := load(source)
	if err != nil {
		return err
	}
	b, err := load(defaults)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(a, b) {
		return fmt.Errorf("deployment: customized profiles differ from package isolation boundaries; explicit migration required")
	}
	return nil
}

// StageManagedUpgrade copies existing registered resources into a private bundle
// for validation. Installation never replaces existing application files.
func StageManagedUpgrade(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "etc/safeterm/config-manager/resources.json"))
	if err != nil {
		return err
	}
	registry, err := configmanager.ParseRegistry(data)
	if err != nil {
		return err
	}
	for _, resource := range registry.Resources {
		if _, err := os.Lstat(resource.Path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := requireSecureFile(resource.Path, false); err != nil {
			return err
		}
		info, err := os.Stat(resource.Path)
		if err != nil {
			return err
		}
		if info.Size() > 4<<20 {
			return fmt.Errorf("deployment: managed resource %s exceeds 4 MiB", resource.ID)
		}
		data, err := os.ReadFile(resource.Path)
		if err != nil {
			return err
		}
		dir := filepath.Join(root, "managed", resource.Profile)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(resource.Path)), data, 0600); err != nil {
			return err
		}
	}
	return nil
}

// InstallConfiguration publishes catalog files while preserving unrelated site
// files, credentials and the Config Manager registry, including their metadata.
// The caller must validate the bundle and hold the installation lifecycle lock.
func InstallConfiguration(source, target string) error {
	sections := []string{"operations", "executables", "profiles", "roles", "policies"}
	for _, dir := range append([]string{"", "trust", "ssh", "console", "config-manager"}, sections...) {
		path := filepath.Join(target, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("deployment: unsafe configuration directory %s", path)
		}
	}
	copyFile := func(name string, preserve bool) error {
		dst := filepath.Join(target, name)
		if info, err := os.Lstat(dst); err == nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("deployment: unsafe configuration target %s", dst)
			}
			if preserve {
				return nil
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(dst), ".install-*")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		mode := os.FileMode(0600)
		if name == "config-manager/resources.json" || name == "ssh/ssh_host_ed25519_key.pub" {
			mode = 0644
		}
		if _, err = file.Write(data); err == nil {
			err = file.Chmod(mode)
		}
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		return os.Rename(file.Name(), dst)
	}
	for _, section := range sections {
		entries, err := os.ReadDir(filepath.Join(source, section))
		if err != nil {
			return err
		}
		keep := map[string]bool{}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if err := copyFile(filepath.Join(section, entry.Name()), false); err != nil {
				return err
			}
			keep[entry.Name()] = true
		}
		old, err := os.ReadDir(filepath.Join(target, section))
		if err != nil {
			return err
		}
		for _, entry := range old {
			if strings.HasSuffix(entry.Name(), ".json") && !keep[entry.Name()] {
				if err := os.Remove(filepath.Join(target, section, entry.Name())); err != nil {
					return err
				}
			}
		}
	}
	for _, name := range []string{"catalog.json", "catalog.sig", "trust/catalog-ed25519.pub", "README.md", "README.txt"} {
		if err := copyFile(name, false); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Join(target, name), 0644); err != nil {
			return err
		}
	}
	for _, section := range sections {
		entries, err := os.ReadDir(filepath.Join(target, section))
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				if err := os.Chmod(filepath.Join(target, section, entry.Name()), 0644); err != nil {
					return err
				}
			}
		}
	}
	for _, name := range []string{"ssh/users.json", "ssh/ssh_host_ed25519_key", "ssh/ssh_host_ed25519_key.pub", "console/users.json", "config-manager/resources.json"} {
		if _, err := os.Lstat(filepath.Join(source, name)); os.IsNotExist(err) {
			continue
		}
		if err := copyFile(name, true); err != nil {
			return err
		}
	}
	return nil
}

// CheckRegistryCatalog verifies cross-file references before any service is stopped.
func CheckRegistryCatalog(cfg *config.Config, registry *configmanager.Registry) error {
	knownRoles := map[string]bool{}
	for _, role := range cfg.Roles {
		knownRoles[role.Name] = true
	}
	operations := map[string]config.OperationDefinition{}
	for _, op := range cfg.Operations {
		operations[op.Name] = op
	}
	for _, resource := range registry.Resources {
		for _, roles := range [][]string{resource.Permissions.ReadRoles, resource.Permissions.ProposeRoles, resource.Permissions.ActivateRoles, resource.Permissions.RollbackRoles} {
			for _, role := range roles {
				if !knownRoles[role] {
					return fmt.Errorf("resource %s references removed role %s", resource.ID, role)
				}
			}
		}
		for _, name := range []string{resource.ActivationOperation, resource.HealthCheckOperation} {
			if name == "" {
				continue
			}
			op, ok := operations[name]
			if !ok || op.ExecutorKind != config.ExecutorSystemd || op.LongRunning || len(op.Parameters) != 0 {
				return fmt.Errorf("resource %s activation %s must remain a bounded argument-free systemd operation", resource.ID, name)
			}
		}
	}
	return nil
}
