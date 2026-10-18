//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"safeterm/internal/resources"
)

func resourceFiles(live string) map[string]string {
	return map[string]string{
		"registry.json": filepath.Join(live, "resources.json"),
		"writer.conf":   writerDropIn,
		"manifest.json": filepath.Join(live, ".resources-manifest.json"),
	}
}
func sortedExtraKeys(m map[string]string) []string {
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func (r *catalogRuntime) snapshotExtras(source, dest string) error {
	for _, name := range sortedExtraKeys(r.extraFiles) {
		path := r.extraFiles[name]
		if source != r.live {
			path = filepath.Join(source, ".resources", name)
		}
		data, err := catalogRead(path)
		if os.IsNotExist(err) && name == "manifest.json" && source == r.live {
			data, err = json.Marshal(resources.EmptyManifest())
		}
		if err != nil {
			return fmt.Errorf("snapshot %s: %w", name, err)
		}
		if err = os.MkdirAll(filepath.Join(dest, ".resources"), 0700); err != nil {
			return err
		}
		if err = catalogAtomicFile(filepath.Join(dest, ".resources", name), data, 0600); err != nil {
			return err
		}
	}
	return nil
}
func (r *catalogRuntime) publishExtras(source string) error {
	if len(r.extraFiles) == 0 {
		return nil
	}
	for _, name := range sortedExtraKeys(r.extraFiles) {
		data, err := catalogRead(filepath.Join(source, ".resources", name))
		if err != nil {
			return err
		}
		if err = catalogAtomicFile(r.extraFiles[name], data, 0644); err != nil {
			return err
		}
	}
	_, err := r.run("/usr/bin/systemctl", "daemon-reload")
	return err
}

// Read once through a non-following fd, checking the object actually read.
func resourceReadRoot(path string) ([]byte, error) {
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
	if !ok || !info.Mode().IsRegular() || stat.Nlink != 1 || stat.Uid != 0 || info.Mode().Perm()&0022 != 0 || info.Size() > resources.MaxFileBytes {
		return nil, fmt.Errorf("%s must be a root-owned, non-writable-by-others, single-link regular file <=1 MiB", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, resources.MaxFileBytes+1))
	if err != nil || len(data) > resources.MaxFileBytes {
		return nil, fmt.Errorf("cannot read bounded resource definition")
	}
	return data, nil
}

func loadResourceDefinitions(dir string, read func(string) ([]byte, error)) ([]resources.Named, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	items := []resources.Named{}
	total := 0
	for _, entry := range entries {
		name := entry.Name()
		// Ignore the exact template before stat/open/decode, regardless of content.
		if name == resources.TemplateName || filepath.Ext(name) != ".json" {
			continue
		}
		if len(items) >= resources.MaxFiles {
			return nil, fmt.Errorf("resources.d exceeds %d active files", resources.MaxFiles)
		}
		data, err := read(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("resources.d/%s: %w", name, err)
		}
		total += len(data)
		if total > 8<<20 {
			return nil, fmt.Errorf("resources.d exceeds 8 MiB")
		}
		item, err := resources.Parse("resources.d/"+name, data)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
