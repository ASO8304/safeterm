//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCatalogInheritedLockHelper(t *testing.T) {
	path := os.Getenv("SAFETERM_CATALOG_LOCK_TEST")
	if path == "" {
		return
	}
	unlock, err := catalogLockAt(path, true)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}

func TestCatalogLockSerializesOperationsAndPreservesInstallerLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lifecycle.lock")
	parent, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err = syscall.Flock(int(parent.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if unlock, err := catalogLockAt(path, false); err == nil {
		unlock()
		t.Fatal("parallel lifecycle mutation acquired lock")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCatalogInheritedLockHelper$")
	cmd.Env = append(os.Environ(), "SAFETERM_CATALOG_LOCK_TEST="+path)
	cmd.ExtraFiles = make([]*os.File, 7)
	cmd.ExtraFiles[6] = parent // inherited descriptor 9, matching the package installer
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("inherited lock failed: %v %s", err, output)
	}
	if unlock, err := catalogLockAt(path, false); err == nil {
		unlock()
		t.Fatal("checkpoint released the parent installer lock")
	}
	if err = syscall.Flock(int(parent.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	unlock, err := catalogLockAt(path, false)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
