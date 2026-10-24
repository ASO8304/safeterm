//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/integrity"
	"safeterm/internal/layout"
	"safeterm/internal/resources"
	"safeterm/internal/siteconfig"
)

type resourceRuntime struct {
	overlays    siteconfig.Inputs
	export      string
	exportCheck func(string) error
	catalog     *catalogRuntime
	defaults    string
	plan        func(*configmanager.Registry, bool) ([]byte, error)
	logs        func(resources.Log) error
	services    func(*resources.Service) error
}

func resourceCommand(dir string, args []string, out io.Writer) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("resource administration requires root; use sudo termctl %s", args[0])
	}
	if dir != defaultConfigDir {
		return fmt.Errorf("resource administration targets /etc/safeterm")
	}
	action := args[0]
	key := "/root/.config/safeterm/catalog-ed25519.key"
	signed := ""
	export := ""
	recovering := false
	rollback := false
	seen := map[string]bool{}
	for i := 1; i < len(args); i++ {
		flag := args[i]
		if seen[flag] {
			return fmt.Errorf("duplicate option %s", flag)
		}
		seen[flag] = true
		switch flag {
		case "--installer-lock-held":
			if action != "initialize" {
				return fmt.Errorf("installer lock inheritance is private to initialize")
			}
		case "--rollback":
			rollback = true
		case "--recover":
			recovering = true
		case "--signed-catalog", "--key-file", "--export":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a directory/file", flag)
			}
			i++
			if flag == "--export" {
				export = args[i]
			} else if flag == "--signed-catalog" {
				signed = args[i]
			} else {
				key = args[i]
			}
		default:
			return fmt.Errorf("unknown %s option %s", action, flag)
		}
	}
	if action == "check" && len(args) != 1 {
		return fmt.Errorf("termctl check takes no options")
	}
	if rollback && len(args) != 2 {
		return fmt.Errorf("use apply --rollback alone")
	}
	if signed != "" && seen["--key-file"] || recovering && len(args) != 2 || export != "" && (signed != "" || seen["--key-file"]) {
		return fmt.Errorf("use one signing method, or apply --recover alone")
	}
	inherited := action == "initialize" && seen["--installer-lock-held"]
	if action == "initialize" && !inherited {
		return fmt.Errorf("initialize is private to the locked installer")
	}
	unlock, err := catalogLock(inherited)
	if err != nil {
		return err
	}
	defer unlock()
	if err = layout.RejectLegacy(dir); err != nil {
		return err
	}
	for _, path := range []string{dir, filepath.Join(dir, "resources.d"), layout.Generated, layout.Defaults, filepath.Dir(writerDropIn), filepath.Dir("/etc/sudoers.d/safeterm-service-manager")} {
		if err = catalogSecureDirectory(path); err != nil {
			return err
		}
	}
	for _, state := range []string{catalogStateDir} {
		if err = os.MkdirAll(state, 0700); err != nil {
			return err
		}
		if err = catalogSecureDirectory(state); err != nil {
			return err
		}
		info, err := os.Stat(state)
		if err != nil || info.Mode().Perm() != 0700 {
			return fmt.Errorf("recovery state must have mode 0700")
		}
	}
	r := &catalogRuntime{managedProfiles: true, live: layout.Generated, state: catalogStateDir, key: key, sudoers: "/etc/sudoers.d/safeterm-service-manager", out: out, run: catalogRun, extraFiles: resourceFiles(layout.Generated)}
	pending := filepath.Join(r.state, "pending")
	if rollback {
		return r.rollback()
	}
	if recovering {
		if _, err = os.Lstat(pending); err != nil {
			return fmt.Errorf("no pending transaction to recover: %w", err)
		}
		if err = r.recoverPending(); err != nil {
			return err
		}
		fmt.Fprintln(out, "Resource activation recovered; prior definitions restored")
		return nil
	}
	if _, err = os.Lstat(pending); err == nil {
		return fmt.Errorf("interrupted activation: run sudo termctl apply --recover")
	} else if !os.IsNotExist(err) {
		return err
	}
	if action == "initialize" {
		// The installer has staged verified package defaults; identities may already
		// use site roles. Full credential validation runs against the compiled result
		// before any service starts, not against this temporary bootstrap baseline.
		if err = r.saveCheckpoint("current", r.live); err != nil {
			return err
		}
		if err = os.RemoveAll(filepath.Join(r.state, "previous")); err != nil {
			return err
		}
	}
	items, err := loadResourceDefinitions(filepath.Join(dir, "resources.d"), resourceReadRoot)
	if err != nil {
		return err
	}
	overlays, err := loadSiteInputs(dir)
	if err != nil {
		return err
	}
	r.managedProfiles = true
	runtime := resourceRuntime{overlays: overlays, defaults: layout.Defaults, export: export, exportCheck: catalogSecureDirectory, catalog: r, plan: configmanager.WriterUnitDropIn, logs: checkResourceLog, services: checkResourceService}
	return runtime.apply(items, action == "check", signed)
}

func checkResourceLog(log resources.Log) error {
	if err := resources.ValidateLogPath(log.Path); err != nil {
		return err
	}
	for path := log.Path; ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("log path must not traverse symlinks")
		}
		if path == log.Path {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || !info.Mode().IsRegular() || stat.Nlink != 1 {
				return fmt.Errorf("log must be a single-link regular file")
			}
		}
		if path == filepath.Dir(path) {
			break
		}
	}
	if _, err := catalogRun("/usr/bin/sudo", "-n", "-u", "safeterm-log-reader", "-g", "safeterm-gateway", "--", "/usr/bin/test", "-r", log.Path); err != nil {
		return fmt.Errorf("read-access check for log %q at %q as safeterm-log-reader failed: %w", log.ID, log.Path, err)
	}
	return nil
}

func (r resourceRuntime) apply(items []resources.Named, check bool, signed string) error {
	c := r.catalog
	builtins, err := config.LoadConfig(r.defaults)
	if err != nil {
		return fmt.Errorf("load packaged built-ins: %w", err)
	}
	if err = integrity.VerifyCatalogFiles(builtins, layout.PublicKey(c.live), filepath.Join(r.defaults, "catalog.sig")); err != nil {
		return err
	}
	baseCfg, err := c.verified(c.live)
	if err != nil {
		return err
	}
	preparedBase, err := siteconfig.Prepare(builtins, r.overlays)
	if err != nil {
		return err
	}
	cfg, reg, next, err := resources.Compile(preparedBase, items)
	if err != nil {
		return err
	}
	if err = siteconfig.Finish(cfg, r.overlays); err != nil {
		return err
	}
	if err = broker.ValidateOperationPaths(cfg); err != nil {
		return err
	}
	plan, err := r.plan(reg, true)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Definition.Service != nil && r.services != nil {
			if err = r.services(item.Definition.Service); err != nil {
				return fmt.Errorf("%s: service: %w", item.File, err)
			}
		}
		for _, log := range item.Definition.Logs {
			if err = r.logs(log); err != nil {
				return fmt.Errorf("%s: logs.%s: %w", item.File, log.ID, err)
			}
		}
	}
	work, err := os.MkdirTemp(c.state, ".resources-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	candidate := filepath.Join(work, "candidate")
	if err = catalogWriteModel(candidate, cfg); err != nil {
		return err
	}
	baseline, old, err := c.baseline()
	if err != nil {
		return err
	}
	regData, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	regData = append(regData, '\n')
	manifestData, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	manifestData = append(manifestData, '\n')
	extras := map[string][]byte{"registry.json": regData, "writer.conf": plan, "manifest.json": manifestData}
	stageExtras := func(dir string) error {
		if err := os.MkdirAll(filepath.Join(dir, ".resources"), 0700); err != nil {
			return err
		}
		for name, data := range extras {
			if len(data) > resources.MaxFileBytes {
				return fmt.Errorf("generated %s exceeds 1 MiB", name)
			}
			if err := catalogAtomicFile(filepath.Join(dir, ".resources", name), data, 0600); err != nil {
				return err
			}
		}
		return nil
	}
	if err = stageExtras(candidate); err != nil {
		return err
	}
	if err = c.validate(cfg, old, candidate); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "Resources: %d definitions, %d managed configs, %d generated operations; rebuilt from packaged built-ins and resources.d only\n", len(items), len(next.Resources), len(next.Operations))
	if check {
		fmt.Fprintln(c.out, "Check: PASS; no active files, permissions or services changed; signing not required")
		return nil
	}
	if r.export != "" {
		if err = r.exportCheck(filepath.Dir(r.export)); err != nil {
			return fmt.Errorf("export needs a root-controlled parent directory: %w", err)
		}
		if err = os.Mkdir(r.export, 0700); err != nil {
			return fmt.Errorf("export requires a new directory: %w", err)
		}
		if err = catalogWriteModel(r.export, cfg); err != nil {
			return err
		}
		if err = os.Mkdir(filepath.Join(r.export, "trust"), 0700); err != nil {
			return err
		}
		trust, err := catalogRead(layout.PublicKey(c.live))
		if err != nil {
			return err
		}
		if err = catalogAtomicFile(filepath.Join(r.export, "trust/catalog-ed25519.pub"), trust, 0644); err != nil {
			return err
		}
		if err = catalogSyncTree(r.export); err != nil {
			return err
		}
		fmt.Fprintf(c.out, "Unsigned candidate exported to %s; sign offline, then apply --signed-catalog DIR; active configuration unchanged\n", r.export)
		return nil
	}

	input := candidate
	signedInput := signed != ""
	if signedInput {
		input = signed
	}
	signingWork := filepath.Join(work, "signed-stage")
	if err = os.Mkdir(signingWork, 0700); err != nil {
		return err
	}
	var prepared *config.Config
	expectedUnsigned, _ := integrity.CanonicalCatalog(cfg)
	activeUnsigned, _ := integrity.CanonicalCatalog(baseCfg)
	if !signedInput && bytes.Equal(expectedUnsigned, activeUnsigned) {
		prepared = cfg
		if err = catalogWriteModel(filepath.Join(signingWork, "candidate"), cfg); err != nil {
			return err
		}
		sig, err := catalogRead(filepath.Join(c.live, "catalog.sig"))
		if err != nil {
			return err
		}
		if err = catalogAtomicFile(filepath.Join(signingWork, "candidate/catalog.sig"), sig, 0644); err != nil {
			return err
		}
	} else {
		prepared, err = c.candidate(input, signingWork, signedInput)
		if err != nil {
			return err
		}
	}
	expected, _ := integrity.CanonicalCatalog(cfg)
	actual, _ := integrity.CanonicalCatalog(prepared)
	if !bytes.Equal(expected, actual) {
		return fmt.Errorf("signed catalog does not match compiled resources; export with termctl apply --export DIR, sign offline and retry")
	}
	candidate = filepath.Join(signingWork, "candidate")
	if err = stageExtras(candidate); err != nil {
		return err
	}
	if err = c.validate(prepared, old, candidate); err != nil {
		return err
	}
	current, _ := integrity.CanonicalCatalog(baseCfg)
	same := bytes.Equal(expected, current)
	for name, want := range extras {
		have, err := catalogRead(c.extraFiles[name])
		if err != nil || !bytes.Equal(have, want) {
			same = false
		}
	}
	desired, err := catalogRead(filepath.Join(candidate, "sudoers"))
	if err != nil {
		return err
	}
	deployed, err := catalogRead(c.sudoers)
	if err != nil || !bytes.Equal(desired, deployed) {
		same = false
	}
	if same {
		fmt.Fprintln(c.out, "Already current: PASS; no services restarted")
		return nil
	}
	if err = c.activate(candidate, baseline); err != nil {
		return err
	}
	fmt.Fprintln(c.out, "Resources applied: PASS; application files, owners and ACLs unchanged; reconnect existing terminal sessions")
	return nil
}

func checkResourceService(service *resources.Service) error {
	data, err := catalogRun("/usr/bin/systemctl", "show", "--property=LoadState", "--value", service.Unit)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) != "loaded" {
		return fmt.Errorf("unit %s is not loaded (missing or masked)", service.Unit)
	}
	return nil
}

// candidate signs only the compiled model or verifies an explicitly supplied
// offline candidate; callers compare it to current compiled resources.
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
