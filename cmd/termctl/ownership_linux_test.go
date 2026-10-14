//go:build linux

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestIdentityReplacementPreservesOwnerGroupAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, gid := range groups {
		if gid != os.Getegid() {
			if err := os.Chown(path, -1, gid); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	before, _ := os.Stat(path)
	if err := writeJSONAtomic(path, map[string]string{"test": "updated"}, 0640); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	want := before.Sys().(*syscall.Stat_t)
	got := after.Sys().(*syscall.Stat_t)
	if got.Uid != want.Uid || got.Gid != want.Gid || after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("identity metadata changed: uid=%d/%d gid=%d/%d mode=%o/%o", got.Uid, want.Uid, got.Gid, want.Gid, after.Mode().Perm(), before.Mode().Perm())
	}
}

func TestIdentityReplacementRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONAtomic(link, map[string]string{}, 0640); err == nil {
		t.Fatal("symlink accepted")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "original" {
		t.Fatal("symlink target changed")
	}
}
