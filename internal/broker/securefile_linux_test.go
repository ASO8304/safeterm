//go:build linux

package broker

import (
	"os"
	"path/filepath"
	"testing"
)

// TEST-SECURITY-010: REQ-014 AC-6.
func TestSecurityFileRejectsHardLinkAlias(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "identity.json")
	if err := os.WriteFile(path, []byte(`{"users":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(directory, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := readSecurityFile(path, "identity", 1024, 0o022); err == nil {
		t.Fatal("hard-linked security file was accepted")
	}
}
