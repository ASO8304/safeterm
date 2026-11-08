package networkmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerRejectsDefaultDeletionAcrossSavedDraftsAndOldCandidates(t *testing.T) {
	m, root, _, backend := managerTest(t)
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", fixture+"      routes:\n        - to: default\n          via: 192.0.2.1\n")
	read := operatorCall(t, m, Request{Action: "read"})
	var name string
	for key := range read.View.Routes {
		name = key
	}
	if name == "" {
		t.Fatal("fixture has no default route")
	}
	request := Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "route", Name: name, Action: "delete"}}}
	if _, err := callTest(m, "admin", []string{"network-operator"}, request); err == nil || !strings.Contains(err.Error(), "default routes cannot be deleted") {
		t.Fatal("manager accepted direct default deletion", err)
	}
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "route", Name: name, Action: "set", Field: "destination", Values: []string{"10.0.0.0/8"}}}})
	read = operatorCall(t, m, Request{Action: "read"})
	path := filepath.Join(root, "state", candidateFile("ssh:admin"))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	request.Version = read.View.Version
	if _, err := callTest(m, "admin", []string{"network-operator"}, request); err == nil || !strings.Contains(err.Error(), "default routes cannot be deleted") {
		t.Fatal("save-then-delete bypassed active default protection", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected save changed the saved candidate", err)
	}
	// Candidates saved by older releases must obey the same rule at apply time.
	var old candidate
	if err := m.load(candidateFile("ssh:admin"), &old); err != nil {
		t.Fatal(err)
	}
	delete(old.View.Routes, name)
	if err := m.save(candidateFile("ssh:admin"), old); err != nil {
		t.Fatal(err)
	}
	if _, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "apply"}); err == nil || !strings.Contains(err.Error(), "default routes cannot be deleted") {
		t.Fatal("old candidate removed a default route", err)
	}
	if backend.trial != nil || m.pending != nil {
		t.Fatal("rejected candidate started a network trial")
	}
}
