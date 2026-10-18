//go:build linux

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"safeterm/internal/resources"
)

// Run only in the disposable CI host, never on real application paths.
func TestResourceRootFileBoundary(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root boundary test runs in isolated CI")
	}
	dir, err := os.MkdirTemp("/var/lib", "safeterm-resources-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err = os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "resource.json")
	data := []byte(`{"id":"output","description":"Output","logs":[{"id":"main","path":"/var/log/output.log","roles":["viewer"]}]}`)
	if err = os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err = catalogSecureDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if _, err = resourceReadRoot(path); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0664, 0666} {
		if err = os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err = resourceReadRoot(path); err == nil {
			t.Fatal("writable-by-others definition accepted")
		}
	}
	_ = os.Chmod(path, 0644)
	if err = os.Chown(path, 65534, 65534); err != nil {
		t.Fatal(err)
	}
	if _, err = resourceReadRoot(path); err == nil {
		t.Fatal("non-root-owned input accepted")
	}
	_ = os.Chown(path, 0, 0)
	hard := filepath.Join(dir, "hard.json")
	if err = os.Link(path, hard); err != nil {
		t.Fatal(err)
	}
	if _, err = resourceReadRoot(path); err == nil {
		t.Fatal("hardlink accepted")
	}
	_ = os.Remove(hard)
	link := filepath.Join(dir, "link.json")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err = resourceReadRoot(link); err == nil {
		t.Fatal("symlink accepted")
	}
	_ = os.Remove(link)
	fifo := filepath.Join(dir, "fifo.json")
	if err = syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = resourceReadRoot(fifo); err == nil {
		t.Fatal("FIFO accepted")
	}
	_ = os.Remove(fifo)
	if err = os.Truncate(path, resources.MaxFileBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err = resourceReadRoot(path); err == nil {
		t.Fatal("oversized input accepted")
	}
	_ = os.Remove(path)
	// Even a dangling symlink named TEMPLATE.json is not opened or decoded.
	if err = os.Symlink("/does/not/exist", filepath.Join(dir, resources.TemplateName)); err != nil {
		t.Fatal(err)
	}
	items, err := loadResourceDefinitions(dir, resourceReadRoot)
	if err != nil || len(items) != 0 {
		t.Fatalf("template was not ignored: %v", err)
	}
	if err = os.Chmod(dir, 0775); err != nil {
		t.Fatal(err)
	}
	if err = catalogSecureDirectory(dir); err == nil {
		t.Fatal("writable input directory accepted")
	}
}
