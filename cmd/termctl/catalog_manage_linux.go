//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/deployment"
	"safeterm/internal/identity"
	"safeterm/internal/integrity"
)

const catalogStateDir = "/var/lib/safeterm-installer/catalog"

type catalogRuntime struct {
	live, state, key, sudoers string
	run                       func(string, ...string) ([]byte, error)
	out                       io.Writer
	registry                  func(string) (*configmanager.Registry, error)
}
type catalogJournal struct {
	Version int      `json:"version"`
	Active  []string `json:"active"`
}

func catalogManageCommand(dir string, args []string, out io.Writer) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("catalog %s requires root; use sudo termctl catalog %s", args[0], args[0])
	}
	if filepath.Clean(dir) != defaultConfigDir {
		return fmt.Errorf("catalog activation always targets /etc/safeterm; use --source DIR for candidate input")
	}
	var source, signed, key string
	var dry, inherited bool
	seenOptions := map[string]bool{}
	for i := 1; i < len(args); i++ {
		if seenOptions[args[i]] {
			return fmt.Errorf("duplicate catalog option %s", args[i])
		}
		seenOptions[args[i]] = true
		switch args[i] {
		case "--dry-run":
			dry = true
		case "--installer-lock-held":
			inherited = true
		case "--source", "--signed-catalog", "--key-file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			flag := args[i]
			i++
			switch flag {
			case "--source":
				source = args[i]
			case "--signed-catalog":
				signed = args[i]
			case "--key-file":
				key = args[i]
			}
		default:
			return fmt.Errorf("unknown catalog option %q", args[i])
		}
	}
	if args[0] != "checkpoint" && inherited {
		return fmt.Errorf("installer lock inheritance is only supported for checkpoint")
	}
	if args[0] != "apply" && (source != "" || signed != "" || key != "" || dry) {
		return fmt.Errorf("catalog %s takes no apply options", args[0])
	}
	if source != "" && signed != "" || signed != "" && key != "" {
		return fmt.Errorf("use --source with local signing, or --signed-catalog without a key")
	}
	if key == "" {
		key = "/root/.config/safeterm/catalog-ed25519.key"
	}
	r := catalogRuntime{live: dir, state: catalogStateDir, key: key, sudoers: "/etc/sudoers.d/safeterm-service-manager", out: out, run: catalogRun}
	unlock, err := catalogLock(inherited)
	if err != nil {
		return err
	}
	defer unlock()
	for _, path := range []string{r.live, filepath.Join(r.live, "trust"), filepath.Dir(r.sudoers)} {
		if err = catalogSecureDirectory(path); err != nil {
			return err
		}
	}
	for _, section := range catalogSections {
		if err = catalogSecureDirectory(filepath.Join(r.live, section)); err != nil {
			return err
		}
	}
	if err = os.MkdirAll(r.state, 0700); err != nil {
		return err
	}
	if err = catalogSecureDirectory(r.state); err != nil {
		return err
	}
	if err = os.Chmod(r.state, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(r.state)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		for _, prefix := range []string{".apply-", ".publish-", ".checkpoint-", ".transaction-", ".rollback-"} {
			if strings.HasPrefix(entry.Name(), prefix) {
				if err = os.RemoveAll(filepath.Join(r.state, entry.Name())); err != nil {
					return err
				}
				break
			}
		}
	}
	if _, err = os.Lstat(filepath.Join(r.state, "pending")); err == nil && args[0] != "rollback" {
		return fmt.Errorf("an interrupted catalog transaction exists; run sudo termctl catalog rollback first")
	}
	switch args[0] {
	case "checkpoint":
		return r.checkpoint()
	case "rollback":
		return r.rollback()
	case "apply":
		if source == "" {
			source = r.live
		}
		if signed != "" {
			source = signed
		}
		return r.apply(source, signed != "", dry)
	}
	return fmt.Errorf("unknown catalog action")
}

func (r *catalogRuntime) verified(dir string) (*config.Config, error) {
	cfg, err := config.LoadConfig(dir)
	if err != nil {
		return nil, err
	}
	if err = integrity.VerifyCatalogFiles(cfg, filepath.Join(r.live, "trust/catalog-ed25519.pub"), filepath.Join(dir, "catalog.sig")); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (r *catalogRuntime) candidate(source, stage string, signed bool) (*config.Config, error) {
	raw := filepath.Join(stage, "input")
	if err := catalogCopyInput(source, raw); err != nil {
		return nil, err
	}
	cfg, err := config.LoadConfig(raw)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(stage, "candidate")
	if err = catalogWriteModel(dir, cfg); err != nil {
		return nil, err
	}
	var signature []byte
	if signed {
		signature, err = catalogRead(filepath.Join(source, "catalog.sig"))
	} else {
		// Existing trust only: no key generation and no implicit trust rotation.
		key, readErr := integrity.ReadPrivateKey(r.key)
		if readErr != nil {
			return nil, fmt.Errorf("no readable existing local signing key; use --signed-catalog DIR or --key-file FILE: %w", readErr)
		}
		defer clear(key)
		stat, statErr := os.Lstat(r.key)
		if statErr != nil || !stat.Mode().IsRegular() {
			return nil, fmt.Errorf("unsafe signing key")
		}
		value, signErr := integrity.SignCatalog(cfg, key)
		err = signErr
		signature = integrity.EncodeBase64(value)
	}
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(dir, "catalog.sig"), signature, 0644); err != nil {
		return nil, err
	}
	if _, err = r.verified(dir); err != nil {
		return nil, fmt.Errorf("candidate does not match installed trust: %w", err)
	}
	return cfg, nil
}

func (r *catalogRuntime) validate(cfg, baseline *config.Config, dir string) error {
	// Catalog apply cannot provision new service accounts, sockets or unit sandboxes.
	profiles := func(c *config.Config) []config.ProfileDefinition {
		v := append([]config.ProfileDefinition(nil), c.Profiles...)
		for i := range v {
			v[i].Capabilities = append([]string(nil), v[i].Capabilities...)
			sort.Strings(v[i].Capabilities)
		}
		sort.Slice(v, func(i, j int) bool { return v[i].Name < v[j].Name })
		return v
	}
	if !reflect.DeepEqual(profiles(cfg), profiles(baseline)) {
		return fmt.Errorf("profile/unit boundary changes require a package deployment")
	}
	for _, p := range cfg.Profiles {
		if _, err := integrity.NewExecutor(cfg, p.Name, catalogVerificationExecutor{}); err != nil {
			return err
		}
	}
	catalog, err := broker.NewCatalog(cfg)
	if err != nil {
		return err
	}
	if _, err = broker.LoadSSHAuthenticator(filepath.Join(r.live, "ssh/users.json"), catalog.KnownRoles()); err != nil {
		return err
	}
	users, err := broker.ReadConsoleUsersData(filepath.Join(r.live, "console/users.json"))
	if err != nil {
		return err
	}
	if _, err = identity.ParseUsers(users, catalog.KnownRoles()); err != nil {
		return err
	}
	loadRegistry := r.registry
	if loadRegistry == nil {
		loadRegistry = configmanager.LoadRegistry
	}
	registry, err := loadRegistry(filepath.Join(r.live, "config-manager/resources.json"))
	if err != nil {
		return err
	}
	operations := map[string]config.OperationDefinition{}
	for _, op := range cfg.Operations {
		operations[op.Name] = op
	}
	for _, resource := range registry.Resources {
		for _, roles := range [][]string{resource.Permissions.ReadRoles, resource.Permissions.ProposeRoles, resource.Permissions.ActivateRoles, resource.Permissions.RollbackRoles} {
			for _, role := range roles {
				if !catalog.KnownRoles()[role] {
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
	lines, err := deployment.CatalogSudoers(cfg)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "sudoers")
	if err = catalogAtomicFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0440); err != nil {
		return err
	}
	_, err = r.run("/usr/sbin/visudo", "-cf", path)
	return err
}

type catalogVerificationExecutor struct{}

func (catalogVerificationExecutor) Execute(context.Context, string, []string) (string, string, int, error) {
	return "", "", -1, fmt.Errorf("verification does not execute commands")
}

func (r *catalogRuntime) snapshot(source, dest string) error {
	cfg, err := r.verified(source)
	if err != nil {
		return err
	}
	if err = catalogWriteModel(dest, cfg); err != nil {
		return err
	}
	data, err := catalogRead(filepath.Join(source, "catalog.sig"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dest, "catalog.sig"), data, 0644); err != nil {
		return err
	}
	lines, err := deployment.CatalogSudoers(cfg)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dest, "sudoers"), []byte(strings.Join(lines, "\n")+"\n"), 0440); err != nil {
		return err
	}
	return catalogSyncTree(dest)
}

func (r *catalogRuntime) saveCheckpoint(name, source string) error {
	temp, err := os.MkdirTemp(r.state, ".checkpoint-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err = r.snapshot(source, temp); err != nil {
		return err
	}
	dest := filepath.Join(r.state, name)
	if err = os.RemoveAll(dest); err != nil {
		return err
	}
	if err = os.Rename(temp, dest); err != nil {
		return err
	}
	return catalogSyncDirectory(r.state)
}

func catalogSyncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func (r *catalogRuntime) checkpoint() error {
	if _, err := r.run("/usr/local/bin/safeterm-preflight", "--installed"); err != nil {
		return err
	}
	if _, err := r.verified(r.live); err != nil {
		return err
	}
	if err := r.saveCheckpoint("current", r.live); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(r.state, "previous")); err != nil {
		return err
	}
	fmt.Fprintln(r.out, "Verified catalog checkpoint saved")
	return nil
}

func (r *catalogRuntime) baseline() (string, *config.Config, error) {
	// Prefer the last activated checkpoint over manually copied/re-signed live files.
	path := filepath.Join(r.state, "current")
	if cfg, err := r.verified(path); err == nil {
		return path, cfg, nil
	}
	cfg, err := r.verified(r.live)
	if err != nil {
		return "", nil, fmt.Errorf("no verified rollback baseline; restore a valid installed catalog before applying changes: %w", err)
	}
	return r.live, cfg, nil
}

func (r *catalogRuntime) apply(source string, signed, dry bool) error {
	work, err := os.MkdirTemp(r.state, ".apply-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	cfg, err := r.candidate(source, work, signed)
	if err != nil {
		return err
	}
	base, old, err := r.baseline()
	if err != nil {
		return err
	}
	candidate := filepath.Join(work, "candidate")
	if err = r.validate(cfg, old, candidate); err != nil {
		return fmt.Errorf("catalog validation failed before activation: %w", err)
	}
	if dry {
		fmt.Fprintln(r.out, "Catalog dry run: PASS (signature, executable hashes, identity/resource roles and sudoers validated; services unchanged)")
		return nil
	}
	if live, err := r.verified(r.live); err == nil {
		current, _ := integrity.CanonicalCatalog(live)
		next, _ := integrity.CanonicalCatalog(cfg)
		deployed, _ := catalogRead(r.sudoers)
		desired, _ := catalogRead(filepath.Join(candidate, "sudoers"))
		if bytes.Equal(current, next) && bytes.Equal(deployed, desired) {
			fmt.Fprintln(r.out, "Catalog already current: PASS; no services restarted")
			return nil
		}
	}
	return r.activate(candidate, base)
}

func (r *catalogRuntime) activeServices() ([]string, error) {
	var active []string
	for _, name := range serviceOrder {
		if name == "audit" {
			continue
		}
		data, err := r.run("/usr/bin/systemctl", "is-active", serviceUnits[name])
		state := strings.TrimSpace(string(data))
		if state == "active" && err == nil {
			active = append(active, name)
			continue
		}
		if state == "inactive" || state == "failed" {
			continue
		}
		return nil, fmt.Errorf("cannot determine service state for %s (%q): %v", name, state, err)
	}
	return active, nil
}

func (r *catalogRuntime) stop(active []string) error {
	var failures []error
	for i := len(active) - 1; i >= 0; i-- {
		if _, err := r.run("/usr/bin/systemctl", "stop", serviceUnits[active[i]]); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
func (r *catalogRuntime) start(active []string) error {
	for _, name := range active {
		if _, err := r.run("/usr/bin/systemctl", "reset-failed", serviceUnits[name]); err != nil {
			return err
		}
		if _, err := r.run("/usr/bin/systemctl", "start", serviceUnits[name]); err != nil {
			return err
		}
	}
	// Type=simple can report active before its process has finished initialization.
	for round := 0; round < 3; round++ {
		if round > 0 {
			time.Sleep(250 * time.Millisecond)
		}
		for _, name := range active {
			if _, err := r.run("/usr/bin/systemctl", "is-active", "--quiet", serviceUnits[name]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *catalogRuntime) publish(source string) error {
	cfg, err := r.verified(source)
	if err != nil {
		return err
	}
	// Stop consumers before replacing this small, fixed set of catalog paths.
	// Never replace /etc/safeterm or its identities/resources/trust directories.
	for _, section := range catalogSections {
		target := filepath.Join(r.live, section)
		entries, err := os.ReadDir(target)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("unsafe live catalog entry")
				}
				if err = os.Remove(filepath.Join(target, entry.Name())); err != nil {
					return err
				}
			}
		}
	}
	work, err := os.MkdirTemp(r.state, ".publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	if err = catalogWriteModel(work, cfg); err != nil {
		return err
	}
	for _, section := range catalogSections {
		data, err := catalogRead(filepath.Join(work, section, section+".json"))
		if err != nil {
			return err
		}
		if err = catalogAtomicFile(filepath.Join(r.live, section, section+".json"), data, 0644); err != nil {
			return err
		}
	}
	for _, name := range []string{"catalog.json", "catalog.sig"} {
		data, err := catalogRead(filepath.Join(source, name))
		if err != nil {
			return err
		}
		if err = catalogAtomicFile(filepath.Join(r.live, name), data, 0644); err != nil {
			return err
		}
	}
	sudoers, err := catalogRead(filepath.Join(source, "sudoers"))
	if err != nil {
		return err
	}
	return catalogAtomicFile(r.sudoers, sudoers, 0440)
}

func (r *catalogRuntime) activate(candidate, base string) error {
	active, err := r.activeServices()
	if err != nil {
		return err
	}
	pending := filepath.Join(r.state, "pending")
	if _, err = os.Lstat(pending); !os.IsNotExist(err) {
		return fmt.Errorf("pending catalog transaction already exists; run catalog rollback")
	}
	// Prepare a complete fsynced journal before the first service/filesystem mutation.
	temp, err := os.MkdirTemp(r.state, ".transaction-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err = r.snapshot(base, filepath.Join(temp, "baseline")); err != nil {
		return err
	}
	data, _ := json.Marshal(catalogJournal{Version: 1, Active: active})
	if err = os.WriteFile(filepath.Join(temp, "state.json"), data, 0600); err != nil {
		return err
	}
	if err = catalogSyncTree(temp); err != nil {
		return err
	}
	if err = os.Rename(temp, pending); err != nil {
		return err
	}
	if err = catalogSyncDirectory(r.state); err != nil {
		return err
	}
	fail := func(cause error) error {
		if recoverErr := r.recoverPending(); recoverErr != nil {
			return fmt.Errorf("activation failed: %v; recovery failed: %w; journal retained, run sudo termctl catalog rollback", cause, recoverErr)
		}
		return fmt.Errorf("activation failed; prior catalog and service state restored: %w", cause)
	}
	if err = r.stop(active); err != nil {
		return fail(err)
	}
	if err = r.publish(candidate); err != nil {
		return fail(err)
	}
	if _, err = r.run("/usr/local/bin/safeterm-preflight", "--installed"); err != nil {
		return fail(err)
	}
	if err = r.start(active); err != nil {
		return fail(err)
	}
	if err = r.saveCheckpoint("previous", filepath.Join(pending, "baseline")); err != nil {
		return fail(err)
	}
	if err = r.saveCheckpoint("current", r.live); err != nil {
		return fail(err)
	}
	if err = os.RemoveAll(pending); err != nil {
		return err
	}
	if err = catalogSyncDirectory(r.state); err != nil {
		return err
	}
	fmt.Fprintln(r.out, "Catalog applied: PASS; prior catalog retained for sudo termctl catalog rollback")
	return nil
}

func (r *catalogRuntime) recoverPending() error {
	pending := filepath.Join(r.state, "pending")
	base := filepath.Join(pending, "baseline")
	if _, err := r.verified(base); err != nil {
		return err
	}
	data, err := catalogRead(filepath.Join(pending, "state.json"))
	if err != nil {
		return err
	}
	var journal catalogJournal
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&journal); err != nil {
		return err
	}
	if journal.Version != 1 {
		return fmt.Errorf("unsupported catalog recovery journal version")
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("invalid catalog recovery journal framing")
	}
	seen := map[string]bool{}
	for _, name := range journal.Active {
		if name == "audit" || serviceUnits[name] == "" || seen[name] {
			return fmt.Errorf("invalid transaction service list")
		}
		seen[name] = true
	}
	if err = r.stop(journal.Active); err != nil {
		return err
	}
	if err = r.publish(base); err != nil {
		return err
	}
	if _, err = r.run("/usr/local/bin/safeterm-preflight", "--installed"); err != nil {
		return err
	}
	if err = r.start(journal.Active); err != nil {
		return err
	}
	if err = r.saveCheckpoint("current", r.live); err != nil {
		return err
	}
	if err = os.RemoveAll(pending); err != nil {
		return err
	}
	return catalogSyncDirectory(r.state)
}

func (r *catalogRuntime) rollback() error {
	if _, err := os.Lstat(filepath.Join(r.state, "pending")); err == nil {
		if err = r.recoverPending(); err != nil {
			return err
		}
		fmt.Fprintln(r.out, "Interrupted catalog transaction recovered: PASS")
		return nil
	}
	previous := filepath.Join(r.state, "previous")
	cfg, err := r.verified(previous)
	if err != nil {
		return fmt.Errorf("no verified previous catalog: %w", err)
	}
	base, old, err := r.baseline()
	if err != nil {
		return err
	}
	// Stage before activation: committing a rollback swaps current/previous.
	work, err := os.MkdirTemp(r.state, ".rollback-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	candidate := filepath.Join(work, "candidate")
	if err = r.snapshot(previous, candidate); err != nil {
		return err
	}
	if err = r.validate(cfg, old, candidate); err != nil {
		return err
	}
	return r.activate(candidate, base)
}
