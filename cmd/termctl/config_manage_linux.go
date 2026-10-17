//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/deployment"
)

const configState = "/var/lib/safeterm-installer/config"
const writerDropIn = "/etc/systemd/system/safeterm-config-writer.service.d/50-resources.conf"

type configRuntime struct {
	registry, dropin, state string
	run                     func(string, ...string) ([]byte, error)
}

func configManageCommand(dir string, args []string, out io.Writer) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("config administration requires root; use sudo termctl config %s", args[0])
	}
	if dir != defaultConfigDir {
		return fmt.Errorf("config administration targets /etc/safeterm")
	}
	inherited := len(args) == 2 && args[0] == "checkpoint" && args[1] == "--installer-lock-held"
	if len(args) != 1 && !inherited {
		return fmt.Errorf("config %s takes no options", args[0])
	}
	unlock, err := catalogLock(inherited)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Stat(filepath.Join(catalogStateDir, "pending")); err == nil {
		return fmt.Errorf("recover pending catalog transaction first")
	}
	r := configRuntime{filepath.Join(dir, "config-manager/resources.json"), writerDropIn, configState, catalogRun}
	for _, path := range []string{filepath.Dir(r.registry), filepath.Dir(r.dropin)} {
		if err := catalogSecureDirectory(path); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(r.state, 0700); err != nil {
		return err
	}
	if err := catalogSecureDirectory(r.state); err != nil {
		return err
	}
	if info, err := os.Stat(r.state); err != nil || info.Mode().Perm() != 0700 {
		return fmt.Errorf("configuration recovery directory must have mode 0700")
	}
	if args[0] == "rollback" {
		if err := r.recover(); err != nil {
			return err
		}
		fmt.Fprintln(out, "configuration transaction recovered")
		return nil
	}
	if _, err := os.Stat(filepath.Join(r.state, "pending")); err == nil {
		return fmt.Errorf("interrupted configuration activation; run sudo termctl config rollback")
	}
	registry, err := configmanager.LoadRegistry(r.registry)
	if err != nil {
		return err
	}
	plan, err := configmanager.WriterUnitDropIn(registry, true)
	if err != nil {
		return err
	}
	cfg, err := config.LoadConfig(dir)
	if err != nil {
		return err
	}
	if err := deployment.CheckRegistryCatalog(cfg, registry); err != nil {
		return err
	}
	if _, err := r.run("/usr/local/bin/safeterm-preflight", "--installed"); err != nil {
		return err
	}
	switch args[0] {
	case "check":
		fmt.Fprintf(out, "configuration valid: %d resources; privileged writer paths verified\n", len(registry.Resources))
		return nil
	case "checkpoint":
		data, err := catalogRead(r.registry)
		if err != nil {
			return err
		}
		return catalogAtomicFile(filepath.Join(r.state, "verified.json"), data, 0600)
	case "apply":
		if err := r.apply(plan); err != nil {
			return err
		}
		fmt.Fprintln(out, "configuration registry and writer sandbox applied; existing file ownership and ACLs unchanged")
		return nil
	default:
		return fmt.Errorf("unknown config administration action")
	}
}

func (r configRuntime) recover() error {
	pending := filepath.Join(r.state, "pending")
	data, err := catalogRead(filepath.Join(pending, "active.json"))
	if err != nil {
		return err
	}
	var active []string
	if err := json.Unmarshal(data, &active); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, name := range []string{"config-writer", "gateway", "config-application", "config-database", "config-network", "ssh"} {
		known[serviceUnits[name]] = true
	}
	for _, unit := range active {
		if !known[unit] {
			return fmt.Errorf("invalid recovery unit")
		}
	}
	registry, err := catalogRead(filepath.Join(pending, "registry.json"))
	if err != nil {
		return err
	}
	if _, err := configmanager.ParseRegistry(registry); err != nil {
		return err
	}
	dropin, err := catalogRead(filepath.Join(pending, "dropin.conf"))
	if err != nil {
		return err
	}
	for i := len(active) - 1; i >= 0; i-- {
		if _, err := r.run("/usr/bin/systemctl", "stop", active[i]); err != nil {
			return err
		}
	}
	if err := catalogAtomicFile(r.registry, registry, 0644); err != nil {
		return err
	}
	if err := catalogAtomicFile(r.dropin, dropin, 0644); err != nil {
		return err
	}
	if _, err := r.run("/usr/bin/systemctl", "daemon-reload"); err != nil {
		return err
	}
	for _, unit := range active {
		if _, err := r.run("/usr/bin/systemctl", "reset-failed", unit); err != nil {
			return err
		}
		if _, err := r.run("/usr/bin/systemctl", "start", unit); err != nil {
			return err
		}
	}
	if err := catalogAtomicFile(filepath.Join(r.state, "verified.json"), registry, 0600); err != nil {
		return err
	}
	return os.RemoveAll(pending)
}

func (r configRuntime) apply(plan []byte) (result error) {
	baseline, err := catalogRead(filepath.Join(r.state, "verified.json"))
	if err != nil {
		return fmt.Errorf("no verified registry checkpoint; reinstall the current package before configuration activation: %w", err)
	}
	if _, err := configmanager.ParseRegistry(baseline); err != nil {
		return err
	}
	oldPlan, err := catalogRead(r.dropin)
	if err != nil {
		return err
	}
	active := []string{}
	for _, name := range []string{"config-writer", "gateway", "config-application", "config-database", "config-network", "ssh"} {
		unit := serviceUnits[name]
		if _, err := r.run("/usr/bin/systemctl", "is-active", "--quiet", unit); err == nil {
			active = append(active, unit)
		}
	}
	stage, err := os.MkdirTemp(r.state, ".prepare-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	data, _ := json.Marshal(active)
	for name, value := range map[string][]byte{"registry.json": baseline, "dropin.conf": oldPlan, "active.json": data} {
		if err := catalogAtomicFile(filepath.Join(stage, name), value, 0600); err != nil {
			return err
		}
	}
	pending := filepath.Join(r.state, "pending")
	if err := os.Rename(stage, pending); err != nil {
		return err
	}
	dir, err := os.Open(r.state)
	if err != nil {
		return err
	}
	err = dir.Sync()
	dir.Close()
	if err != nil {
		return err
	}
	defer func() {
		if result != nil {
			if err := r.recover(); err != nil {
				result = fmt.Errorf("%v; recovery failed: %w; run sudo termctl config rollback", result, err)
			}
		}
	}()
	for i := len(active) - 1; i >= 0; i-- {
		if _, err := r.run("/usr/bin/systemctl", "stop", active[i]); err != nil {
			return err
		}
	}
	if err := catalogAtomicFile(r.dropin, plan, 0644); err != nil {
		return err
	}
	if _, err := r.run("/usr/bin/systemctl", "daemon-reload"); err != nil {
		return err
	}
	for _, unit := range active {
		if _, err := r.run("/usr/bin/systemctl", "reset-failed", unit); err != nil {
			return err
		}
		if _, err := r.run("/usr/bin/systemctl", "start", unit); err != nil {
			return err
		}
		if _, err := r.run("/usr/bin/systemctl", "is-active", "--quiet", unit); err != nil {
			return err
		}
	}
	data, err = catalogRead(r.registry)
	if err != nil {
		return err
	}
	if err := catalogAtomicFile(filepath.Join(r.state, "verified.json"), data, 0600); err != nil {
		return err
	}
	return os.RemoveAll(pending)
}
