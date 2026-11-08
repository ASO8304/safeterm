package networkmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNICRemovalDoesNotBlockOtherIdentityOrPresentDevice(t *testing.T) {
	m, root, p, _ := managerTest(t)
	p.AllowAllInterfaces = true
	m.opts.Policy = func() (Policy, error) { return p, nil }
	live := map[string]string{"ens19": "00:11:22:33:44:55", "ens37": "00:11:22:33:44:66"}
	m.opts.Interfaces = func() map[string]string { return live }
	// A removed device's invalid legacy route must not block a present NIC.
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", fixture+"      routes:\n        - to: default\n          via: 192.0.2.1/24\n")
	read, err := callTest(m, "alice", []string{"network-operator"}, Request{Action: "read"})
	if err != nil {
		t.Fatal(err)
	}
	// Use an interface-only fixture while Alice saves a candidate.
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", fixture)
	read, err = callTest(m, "alice", []string{"network-operator"}, Request{Action: "read"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = callTest(m, "alice", []string{"network-operator"}, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens19", Action: "set", Field: "mtu", Values: []string{"1400"}}}})
	if err != nil {
		t.Fatal(err)
	}
	delete(live, "ens19")
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", fixture+"      routes:\n        - to: default\n          via: 192.0.2.1/24\n")
	read = operatorCall(t, m, Request{Action: "read"})
	if _, exists := read.View.Interfaces["ens19"]; exists || len(read.View.Routes) != 0 {
		t.Fatal("removed NIC or its routes still listed", read.View)
	}
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens37", Action: "set", Field: "dhcp4", Values: []string{"enable"}}}})
	operatorCall(t, m, Request{Action: "discard"})
	// Alice can reopen despite the stale saved draft and replace it explicitly.
	read, err = callTest(m, "alice", []string{"network-operator"}, Request{Action: "read"})
	if err != nil || !strings.Contains(read.Text, "retained") {
		t.Fatal(read, err)
	}
	_, err = callTest(m, "alice", []string{"network-operator"}, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens37", Action: "set", Field: "mtu", Values: []string{"1450"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "state", "stale-"+candidateFile("ssh:alice"))); err != nil {
		t.Fatal("stale draft not retained", err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if !strings.Contains(string(data), "via: 192.0.2.1/24") {
		t.Fatal("removed device source was rewritten")
	}
}

func TestLegacyCandidateMigratesOnlyForItsOwner(t *testing.T) {
	m, root, _, _ := managerTest(t)
	read := operatorCall(t, m, Request{Action: "read"})
	legacy := candidate{Owner: "ssh:alice", Base: read.View.Version, Revision: "legacy", View: *read.View}
	legacy.View.Version = "legacy"
	if err := m.save("candidate.json", legacy); err != nil {
		t.Fatal(err)
	}
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens19", Action: "set", Field: "mtu", Values: []string{"1400"}}}})
	if _, err := os.Stat(filepath.Join(root, "state/candidate.json")); err != nil {
		t.Fatal("another identity's legacy draft lost", err)
	}
	got, err := callTest(m, "alice", []string{"network-operator"}, Request{Action: "read"})
	if err != nil || got.View.Version != "legacy" {
		t.Fatal(got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "state", candidateFile("ssh:alice"))); err != nil {
		t.Fatal(err)
	}
}

func TestRemovedMACDefinitionDoesNotAttachToReplacementNICWithSameName(t *testing.T) {
	root, p := testTree(t)
	p.AllowAllInterfaces = true
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", "network:\n  version: 2\n  ethernets:\n    ens19:\n      match: {macaddress: '00:11:22:33:44:55'}\n      addresses: [192.0.2.10/24]\n")
	s, err := Discover(root, p, map[string]string{"ens19": "00:11:22:33:44:66"})
	if err != nil {
		t.Fatal(err)
	}
	if i := s.View.Interfaces["ens19"]; len(i.Addresses) != 0 || i.ID == "ens19" {
		t.Fatal("replacement inherited removed NIC configuration", i)
	}
	// An unsupported match must still protect newly discovered hardware rather
	// than silently treating a potentially matched device as unconfigured.
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", "network:\n  version: 2\n  ethernets:\n    existing:\n      match: {driver: vmxnet3}\n      dhcp4: true\n")
	s, err = Discover(root, p, map[string]string{"ens19": "00:11:22:33:44:66"})
	if err != nil || s.View.Interfaces["ens19"].ReadOnly == "" {
		t.Fatal("unsupported existing match became editable", s, err)
	}
}

func TestDiscardAndLiveStatusSurviveDamagedSourceAndOwnDraft(t *testing.T) {
	m, root, _, _ := managerTest(t)
	writeTest(t, root, "state/"+candidateFile("ssh:admin"), "not-json")
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", "not: [valid")
	operatorCall(t, m, Request{Action: "status"})
	operatorCall(t, m, Request{Action: "discard"})
	if _, err := os.Stat(filepath.Join(root, "state", candidateFile("ssh:admin"))); !os.IsNotExist(err) {
		t.Fatal("damaged draft could not be discarded", err)
	}
}
