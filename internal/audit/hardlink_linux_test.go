//go:build linux

package audit

import (
	"os"
	"path/filepath"
	"testing"
)

// TEST-SECURITY-009: REQ-014 AC-6 and AC-9.
func TestAuditStoreRejectsHardLinkAlias(t *testing.T) {
	directory := t.TempDir()
	store := filepath.Join(directory, "audit.jsonl")
	if err := os.WriteFile(store, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(store, filepath.Join(directory, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFileStore(store); err == nil {
		t.Fatal("hard-linked audit store was accepted")
	}
}
