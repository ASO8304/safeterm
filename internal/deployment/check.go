package deployment

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/identity"
	"safeterm/internal/integrity"
)

var RuntimeBinaries = []string{
	"safeterm-audit", "safeterm-config-manager", "safeterm-gateway",
	"safeterm-console", "safeterm-runner", "safeterm-ssh-broker", "safeterm-preflight",
	"safeterm-termctl", "safeterm-identity",
}

var SystemdUnits = []string{
	"safeterm-audit.service", "safeterm-identity.service",
	"safeterm-config-manager-application.service",
	"safeterm-config-manager-database.service",
	"safeterm-config-manager-network.service",
	"safeterm-gateway.service",
	"safeterm-runner-diagnostics.service",
	"safeterm-runner-log-reader.service",
	"safeterm-runner-network-observer.service",
	"safeterm-runner-service-manager.service",
	"safeterm-ssh-broker.service",
}

// SystemdSudoers returns the exact root commands required by signed, typed
// systemd operations. It never emits caller-provided parameters or wildcards.
func SystemdSudoers(root string) ([]string, error) {
	root = filepath.Clean(root)
	configDir := filepath.Join(root, "etc", "safeterm")
	if root == "/" {
		configDir = "/etc/safeterm"
	}
	cfg, err := config.LoadConfig(configDir)
	if err != nil {
		return nil, err
	}
	return systemdSudoers(cfg)
}

func systemdSudoers(cfg *config.Config) ([]string, error) {
	commands := []string{}
	for _, operation := range cfg.Operations {
		if operation.ExecutorKind == config.ExecutorSystemd {
			commands = append(commands, "/usr/bin/systemctl --no-pager "+operation.Systemd.Action+" "+operation.Systemd.Unit)
		}
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("deployment: catalog has no typed systemd operations")
	}
	sort.Strings(commands)
	return []string{
		"Cmnd_Alias SAFETERM_SYSTEMD = " + strings.Join(commands, ", "),
		"safeterm-service-manager ALL=(root) NOPASSWD: SAFETERM_SYSTEMD",
	}, nil
}

type CheckOptions struct {
	Root      string
	Installed bool
}

type Report struct {
	CatalogVersion string
	Profiles       int
	Operations     int
	Executables    int
	Resources      int
	Units          int
}

type ManagedFile struct {
	Source string
	Target string
	User   string
}

func Check(options CheckOptions) (Report, error) {
	root := filepath.Clean(options.Root)
	if root == "" || root == "." {
		return Report{}, fmt.Errorf("deployment: bundle root is required")
	}
	if !options.Installed {
		if err := verifyManifest(root); err != nil {
			return Report{}, err
		}
		if err := rejectUnexpectedPrivateKeys(filepath.Join(root, "etc", "safeterm")); err != nil {
			return Report{}, err
		}
	}
	binDir := filepath.Join(root, "bin")
	configDir := filepath.Join(root, "etc", "safeterm")
	unitDir := filepath.Join(root, "systemd")
	if options.Installed {
		binDir = "/usr/local/bin"
		configDir = "/etc/safeterm"
		unitDir = "/etc/systemd/system"
	}
	for _, name := range RuntimeBinaries {
		if err := requireSecureFile(filepath.Join(binDir, name), true); err != nil {
			return Report{}, err
		}
	}
	for _, name := range SystemdUnits {
		path := filepath.Join(unitDir, name)
		if err := requireSecureFile(path, false); err != nil {
			return Report{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Report{}, err
		}
		if !strings.Contains(string(data), "NoNewPrivileges=") || !strings.Contains(string(data), "MemoryMax=") || !strings.Contains(string(data), "TasksMax=") {
			return Report{}, fmt.Errorf("deployment: unit %q lacks required hardening", name)
		}
	}
	cfg, err := config.LoadConfig(configDir)
	if err != nil {
		return Report{}, err
	}
	if err := integrity.VerifyCatalogFiles(cfg, filepath.Join(configDir, "trust", "catalog-ed25519.pub"), filepath.Join(configDir, "catalog.sig")); err != nil {
		return Report{}, err
	}
	if options.Installed {
		expected, err := systemdSudoers(cfg)
		if err != nil {
			return Report{}, err
		}
		path := "/etc/sudoers.d/safeterm-service-manager"
		if err := requireSecureFile(path, false); err != nil {
			return Report{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(data)) != strings.Join(expected, "\n") {
			return Report{}, fmt.Errorf("deployment: installed service-manager sudoers policy does not match signed catalog")
		}
	}
	for _, profile := range cfg.Profiles {
		if _, err := integrity.NewExecutor(cfg, profile.Name, verificationExecutor{}); err != nil {
			return Report{}, fmt.Errorf("deployment: verify profile %q executables: %w", profile.Name, err)
		}
	}
	catalog, err := broker.NewCatalog(cfg)
	if err != nil {
		return Report{}, err
	}
	if _, err := broker.LoadSSHAuthenticator(filepath.Join(configDir, "ssh", "users.json"), catalog.KnownRoles()); err != nil {
		return Report{}, err
	}
	if _, err := broker.LoadSSHHostKey(filepath.Join(configDir, "ssh", "ssh_host_ed25519_key")); err != nil {
		return Report{}, err
	}
	consolePath := filepath.Join(configDir, "console", "users.json")
	if options.Installed {
		group, err := user.LookupGroup(identity.UserName)
		if err != nil {
			return Report{}, err
		}
		gid, err := strconv.ParseUint(group.Gid, 10, 32)
		if err != nil {
			return Report{}, err
		}
		if err = identity.CheckRootManaged(consolePath, uint32(gid)); err != nil {
			return Report{}, err
		}
	}
	consoleData, err := broker.ReadConsoleUsersData(consolePath)
	if err != nil {
		return Report{}, err
	}
	if _, err := identity.ParseUsers(consoleData, catalog.KnownRoles()); err != nil {
		return Report{}, err
	}
	registryPath := filepath.Join(configDir, "config-manager", "resources.json")
	var registry *configmanager.Registry
	if options.Installed {
		registry, err = configmanager.LoadRegistry(registryPath)
	} else {
		var data []byte
		data, err = os.ReadFile(registryPath)
		if err == nil {
			registry, err = configmanager.ParseRegistry(data)
		}
	}
	if err != nil {
		return Report{}, err
	}
	if !options.Installed {
		if _, err := managedFiles(root, registry); err != nil {
			return Report{}, err
		}
	}
	if options.Installed {
		if err := checkAccounts(cfg); err != nil {
			return Report{}, err
		}
		for _, resource := range registry.Resources {
			if err := requireSecureFile(resource.Path, false); err != nil {
				return Report{}, fmt.Errorf("deployment: managed resource %q: %w", resource.ID, err)
			}
		}
	}
	return Report{CatalogVersion: cfg.Version, Profiles: len(cfg.Profiles), Operations: len(cfg.Operations), Executables: len(cfg.Executables), Resources: len(registry.Resources), Units: len(SystemdUnits)}, nil
}

func rejectUnexpectedPrivateKeys(configDir string) error {
	allowed := filepath.Clean(filepath.Join(configDir, "ssh", "ssh_host_ed25519_key"))
	return filepath.WalkDir(configDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Clean(path) == allowed {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("deployment: unsafe configuration entry %q", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 4<<20 {
			return fmt.Errorf("deployment: configuration file %q exceeds 4 MiB", path)
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".key") || strings.HasSuffix(name, ".private-key") || strings.Contains(name, "signing") {
			return fmt.Errorf("deployment: unexpected private-key material %q", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "PRIVATE KEY-----") {
			return fmt.Errorf("deployment: unexpected private-key material %q", path)
		}
		return nil
	})
}

// ManagedFiles returns the exact, preflight-validated bundle source and
// registered target for each managed resource. It is intentionally available
// only for offline bundle roots; installed paths are checked by Check.
func ManagedFiles(root string) ([]ManagedFile, error) {
	registryData, err := os.ReadFile(filepath.Join(root, "etc", "safeterm", "config-manager", "resources.json"))
	if err != nil {
		return nil, err
	}
	registry, err := configmanager.ParseRegistry(registryData)
	if err != nil {
		return nil, err
	}
	return managedFiles(filepath.Clean(root), registry)
}

func managedFiles(root string, registry *configmanager.Registry) ([]ManagedFile, error) {
	users := map[string]string{
		"application": "safeterm-config-application",
		"database":    "safeterm-config-database",
		"network":     "safeterm-config-network",
	}
	result := make([]ManagedFile, 0, len(registry.Resources))
	seenSources := map[string]bool{}
	for _, resource := range registry.Resources {
		owner := users[resource.Profile]
		if owner == "" {
			return nil, fmt.Errorf("deployment: unsupported Config Manager profile %q", resource.Profile)
		}
		source := filepath.Join(root, "managed", resource.Profile, filepath.Base(resource.Path))
		if seenSources[source] {
			return nil, fmt.Errorf("deployment: managed bundle path collision for %q", source)
		}
		seenSources[source] = true
		if err := requireSecureFile(source, false); err != nil {
			return nil, fmt.Errorf("deployment: managed resource %q: %w", resource.ID, err)
		}
		result = append(result, ManagedFile{Source: source, Target: resource.Path, User: owner})
	}
	return result, nil
}

type verificationExecutor struct{}

func (verificationExecutor) Execute(context.Context, string, []string) (string, string, int, error) {
	return "", "", 0, nil
}

func requireSecureFile(path string, executable bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("deployment: inspect %q: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("deployment: %q must be a non-writable regular file", path)
	}
	if executable && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("deployment: %q must be executable", path)
	}
	return nil
}

func checkAccounts(cfg *config.Config) error {
	names := []string{"safeterm-identity", "safeterm", "safeterm-audit", "safeterm-broker", "safeterm-gateway", "safeterm-config-application", "safeterm-config-database", "safeterm-config-network"}
	for _, profile := range cfg.Profiles {
		names = append(names, profile.RunAsUser)
	}
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if _, err := user.Lookup(name); err != nil {
			return fmt.Errorf("deployment: required OS user %q is missing: %w", name, err)
		}
	}
	return nil
}
