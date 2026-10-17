//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"safeterm/internal/config"
)

var catalogSections = []string{"operations", "roles", "policies", "profiles", "executables"}

// Copy only catalog input, never identity maps, private keys or application data.
// O_NOFOLLOW, fstat and bounded reads protect privileged staging from links/FIFOs.
func catalogRead(path string) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Nlink != 1 || info.Size() > 1<<20 {
		return nil, fmt.Errorf("catalog: unsafe or oversized file %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, fmt.Errorf("catalog: cannot read bounded file %s", path)
	}
	return data, nil
}

func catalogCopyInput(source, target string) error {
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}
	data, err := catalogRead(filepath.Join(source, "catalog.json"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(target, "catalog.json"), data, 0644); err != nil {
		return err
	}
	total, count := len(data), 0
	for _, section := range catalogSections {
		dir := filepath.Join(source, section)
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("catalog: unsafe section %s", section)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		if err = os.Mkdir(filepath.Join(target, section), 0755); err != nil {
			return err
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			count++
			if count > 1024 {
				return fmt.Errorf("catalog: too many files")
			}
			data, err := catalogRead(filepath.Join(dir, entry.Name()))
			if err != nil {
				return err
			}
			total += len(data)
			if total > 8<<20 {
				return fmt.Errorf("catalog: catalog exceeds 8 MiB")
			}
			if err = os.WriteFile(filepath.Join(target, section, entry.Name()), data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func catalogWriteModel(dir string, cfg *config.Config) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	values := map[string]any{"catalog.json": map[string]string{"version": cfg.Version},
		"operations/operations.json": cfg.Operations, "roles/roles.json": cfg.Roles,
		"policies/policies.json": cfg.Policies, "profiles/profiles.json": cfg.Profiles, "executables/executables.json": cfg.Executables}
	for name, value := range values {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			return err
		}
	}
	return nil
}

func catalogSyncTree(dir string) error {
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return f.Sync()
	})
}

func catalogAtomicFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".catalog-write-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	parent, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

func catalogSecureDirectory(path string) error {
	for dir := filepath.Clean(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || stat.Uid != 0 || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("catalog: %s must be a root-owned directory without group/world write", dir)
		}
		if dir == filepath.Dir(dir) {
			return nil
		}
	}
}

func catalogLock(inherited bool) (func(), error) {
	return catalogLockAt("/run/lock/safeterm-lifecycle.lock", inherited)
}

func catalogLockAt(path string, inherited bool) (func(), error) {
	if inherited {
		f := os.NewFile(9, path)
		if f == nil {
			return nil, fmt.Errorf("catalog: missing installer lock")
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		expected, err := os.Lstat(path)
		if err != nil || !os.SameFile(info, expected) || !info.Mode().IsRegular() {
			f.Close()
			return nil, fmt.Errorf("catalog: invalid installer lock")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 || info.Mode().Perm()&0022 != 0 {
			f.Close()
			return nil, fmt.Errorf("catalog: unsafe installer lock ownership or permissions")
		}
		syscall.CloseOnExec(9)
		if err = syscall.Flock(9, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			f.Close()
			return nil, err
		}
		// Closing only this inherited descriptor leaves the parent's lock intact.
		return func() { _ = f.Close() }, nil
	}
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 || info.Mode().Perm()&0022 != 0 {
		f.Close()
		return nil, fmt.Errorf("catalog: unsafe lifecycle lock ownership or permissions")
	}
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another SafeTerm lifecycle operation is running")
	}
	return func() { _ = syscall.Flock(fd, syscall.LOCK_UN); _ = f.Close() }, nil
}
