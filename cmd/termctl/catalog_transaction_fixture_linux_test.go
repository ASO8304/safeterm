//go:build linux

package main

// Test-only candidate adapter exercises the transaction engine with synthetic
// signed catalogs. No raw catalog activation entry point is shipped.
import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"safeterm/internal/deployment"
	"safeterm/internal/integrity"
)

func (r *catalogRuntime) apply(source string, signed, dry bool) error {
	work, err := os.MkdirTemp(r.state, ".apply-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	if r.refreshHashes {
		refreshed := filepath.Join(work, "refreshed")
		if err := deployment.PrepareConfig(source, refreshed); err != nil {
			return err
		}
		source = refreshed
	}
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
