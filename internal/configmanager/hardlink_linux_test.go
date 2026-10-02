//go:build linux

package configmanager

import (
	"os"
	"path/filepath"
	"testing"
)

// TEST-SECURITY-008: REQ-014 AC-6.
func TestManagerRejectsHardLinkedManagedFile(t *testing.T) {
	path := writeTestINI(t)
	alias := filepath.Join(filepath.Dir(path), "alias.ini")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager("application", testRegistry(path, false), nil, nil, &recordingSink{}, ManagerOptions{}); err == nil {
		t.Fatal("hard-linked managed file was accepted")
	}
}
