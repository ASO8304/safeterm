package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"safeterm/internal/config"
	"safeterm/internal/deployment"
	"safeterm/internal/integrity"
	"safeterm/internal/layout"
	"safeterm/internal/siteconfig"
)

func defaultsCommand(dir string, args []string, out io.Writer) error {
	if len(args) != 1 && len(args) != 2 {
		return fmt.Errorf("usage: termctl defaults role|operation|profile|executable|policy NAME")
	}
	cfg, err := config.LoadConfig(layout.Defaults)
	if err != nil {
		return err
	}
	if err = integrity.VerifyCatalogFiles(cfg, layout.PublicKey(layout.Catalog(dir)), filepath.Join(layout.Defaults, "catalog.sig")); err != nil {
		return err
	}
	if len(args) == 1 {
		names := []string{}
		switch args[0] {
		case "role":
			for _, v := range cfg.Roles {
				names = append(names, v.Name)
			}
		case "operation":
			for _, v := range cfg.Operations {
				names = append(names, v.Name)
			}
		case "profile":
			for _, v := range cfg.Profiles {
				names = append(names, string(v.Name))
			}
		case "executable":
			for _, v := range cfg.Executables {
				names = append(names, v.ID)
			}
		case "policy":
			for _, v := range cfg.Policies {
				names = append(names, v.Role)
			}
		default:
			return fmt.Errorf("unknown default definition type %s", args[0])
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintln(out, name)
		}
		return nil
	}
	value, err := defaultDefinition(cfg, args[0], args[1])
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
func defaultDefinition(cfg *config.Config, kind, name string) (any, error) {
	switch kind {
	case "role":
		for _, r := range cfg.Roles {
			if r.Name == name {
				return r, nil
			}
		}
	case "operation":
		for _, op := range cfg.Operations {
			if op.Name == name {
				return siteconfig.Operation{Operation: op}, nil
			}
		}
	case "profile":
		for _, p := range cfg.Profiles {
			if string(p.Name) == name {
				return siteconfig.Profile{Name: name, Base: p.Name, MemoryLimitMiB: p.MemoryLimitMiB, ProcessLimit: p.ProcessLimit, TimeoutSeconds: defaultTimeout(p.Name), OutputLimitBytes: 1048576, MaxSessions: 64, IdleTimeoutSeconds: 300}, nil
			}
		}
	case "executable":
		for _, e := range cfg.Executables {
			if e.ID == name {
				return e, nil
			}
		}
	case "policy":
		for _, p := range cfg.Policies {
			if p.Role == name {
				return p, nil
			}
		}
	}
	return nil, fmt.Errorf("no packaged %s named %s", kind, name)
}
func profileCommand(dir string, args []string, out io.Writer) error {
	cfg, err := config.LoadConfig(layout.Catalog(dir))
	if err != nil {
		return err
	}
	if len(args) == 0 || len(args) == 1 && args[0] == "list" {
		for _, p := range cfg.Profiles {
			fmt.Fprintf(out, "%-24s template=%s memory=%dMiB processes=%d\n", p.Name, siteconfig.Base(p), p.MemoryLimitMiB, p.ProcessLimit)
		}
		return nil
	}
	if len(args) == 2 && args[0] == "describe" {
		for _, p := range cfg.Profiles {
			if string(p.Name) == args[1] {
				encoder := json.NewEncoder(out)
				encoder.SetIndent("", "  ")
				return encoder.Encode(p)
			}
		}
	}
	return fmt.Errorf("usage: termctl profile list|describe NAME")
}
func executableCommand(dir string, args []string, out io.Writer) error {
	if len(args) == 2 && args[0] == "hash" {
		digest, err := deployment.ExecutableSHA256(args[1])
		if err != nil {
			return err
		}
		fmt.Fprintln(out, digest)
		return nil
	}
	cfg, err := config.LoadConfig(layout.Catalog(dir))
	if err != nil {
		return err
	}
	if len(args) == 0 || len(args) == 1 && args[0] == "list" {
		for _, e := range cfg.Executables {
			fmt.Fprintf(out, "%-24s %s\n", e.ID, e.Path)
		}
		return nil
	}
	if len(args) == 2 && args[0] == "describe" {
		for _, e := range cfg.Executables {
			if e.ID == args[1] {
				encoder := json.NewEncoder(out)
				encoder.SetIndent("", "  ")
				return encoder.Encode(e)
			}
		}
	}
	return fmt.Errorf("usage: termctl executable list|describe ID | hash PATH")
}

func defaultTimeout(p config.RunnerProfile) int {
	if p == config.ProfileLogReader || p == config.ProfileNetworkObserver {
		return 1800
	}
	return 30
}
