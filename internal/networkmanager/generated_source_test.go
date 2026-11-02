package networkmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestRenamedNewInterfaceRemainsEditableAfterConfirmation(t *testing.T) {
	m, root, p, _ := managerTest(t)
	p.AllowAllInterfaces = true
	m.opts.Policy = func() (Policy, error) { return p, nil }
	m.opts.Interfaces = func() map[string]string {
		return map[string]string{"ens19": "00:11:22:33:44:55", "ens37": "00:0c:29:ef:e4:5b"}
	}
	persistNameTest(t, m, "ens37", "ip1")
	data, err := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	read := operatorCall(t, m, Request{Action: "read"})
	_, err = callTest(m, "admin", []string{"network-operator"}, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens37", Action: "set", Field: "addresses", Values: []string{"192.168.222.99/24"}}}})
	if err != nil {
		t.Fatalf("generated source cannot be edited: %v\n%s", err, data)
	}
	operatorCall(t, m, Request{Action: "apply"})
	operatorCall(t, m, Request{Action: "confirm"})
	read = operatorCall(t, m, Request{Action: "read"})
	if len(read.View.Interfaces["ens37"].Addresses) != 1 || read.View.Interfaces["ens37"].PersistentName != "ip1" {
		t.Fatal("editing generated source lost its address or future name", read.View)
	}
}

func TestLegacyInlineGeneratedSourceCanBeEditedAndRolledBack(t *testing.T) {
	m, root, p, _ := managerTest(t)
	p.AllowAllInterfaces = true
	m.opts.Policy = func() (Policy, error) { return p, nil }
	m.opts.Interfaces = func() map[string]string {
		return map[string]string{"ens19": "00:11:22:33:44:55", "ens37": "00:0c:29:ef:e4:5b"}
	}
	original := "# Managed by SafeTerm for newly discovered interfaces.\nnetwork:\n  version: 2\n  renderer: networkd\n  # Interface definitions are added after a confirmed trial.\n  ethernets: {safeterm-000c29efe45b: {dhcp4: false, match: {macaddress: '00:0c:29:ef:e4:5b'}}}\n"
	writeTest(t, root, managedInterfaces, original)
	if err := prepareNamingDirectory(root); err != nil {
		t.Fatal(err)
	}
	writeTest(t, root, "etc/systemd/network/11-persistent-net.link", "[Match]\nMACAddress=00:0c:29:ef:e4:5b\n[Link]\nName=ip1\n")
	read := operatorCall(t, m, Request{Action: "read"})
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens37", Action: "set", Field: "addresses", Values: []string{"192.168.222.99/24"}}}})
	if data, _ := os.ReadFile(filepath.Join(root, managedInterfaces)); string(data) != original {
		t.Fatal("save rewrote the active file")
	}
	operatorCall(t, m, Request{Action: "apply"})
	operatorCall(t, m, Request{Action: "confirm"})
	read = operatorCall(t, m, Request{Action: "read"})
	item := read.View.Interfaces["ens37"]
	if item.PersistentName != "ip1" || len(item.Addresses) != 1 || item.Addresses[0] != "192.168.222.99/24" {
		t.Fatal("inline edit lost interface values", item)
	}
	data, _ := os.ReadFile(filepath.Join(root, managedInterfaces))
	if !strings.Contains(string(data), "# Managed by SafeTerm") || strings.Contains(string(data), "ethernets: {") {
		t.Fatal("source did not become readable block YAML", string(data))
	}
	operatorCall(t, m, Request{Action: "rollback"})
	if data, _ := os.ReadFile(filepath.Join(root, managedInterfaces)); string(data) != original {
		t.Fatal("rollback did not restore the original inline bytes")
	}
}

func TestNewInterfaceDestinationMustBeExistingAndUnambiguous(t *testing.T) {
	root, p := testTree(t)
	p.AllowAllInterfaces = true
	names := map[string]string{"ens19": "00:11:22:33:44:55", "ens37": "00:0c:29:ef:e4:5b"}
	writeTest(t, root, "etc/netplan/01-primary.yaml", "# Primary source\nnetwork:\n  version: 2\n  renderer: networkd\n")
	snapshot, err := Discover(root, p, names)
	if err != nil || !strings.Contains(snapshot.View.Interfaces["ens37"].ReadOnly, "new_interface_file") {
		t.Fatal("ambiguous source was guessed", snapshot, err)
	}
	p.NewInterfaceFile = "01-primary.yaml"
	snapshot, err = Discover(root, p, names)
	if err != nil || snapshot.View.Interfaces["ens37"].ReadOnly != "" || snapshot.View.Interfaces["ens37"].Files[0] != "etc/netplan/01-primary.yaml" {
		t.Fatal("explicit destination ignored", snapshot, err)
	}
	modified, err := Modify(snapshot.View, []Change{{Kind: "interface", Name: "ens37", Field: "dhcp4", Action: "set", Values: []string{"enable"}}})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := snapshot.Render(modified)
	if err != nil || len(changed) != 1 {
		t.Fatal(changed, err)
	}
	file := File{Path: "etc/netplan/01-primary.yaml", Data: changed["etc/netplan/01-primary.yaml"]}
	if err := parseFile(&file); err != nil {
		t.Fatal(err)
	}
	ether := child(child(file.doc.Content[0], "network"), "ethernets")
	if ether == nil || ether.Style&yaml.FlowStyle != 0 {
		t.Fatal("new entries were generated inline")
	}
	p.NewInterfaceFile = "missing.yaml"
	snapshot, err = Discover(root, p, names)
	if err != nil || snapshot.View.Interfaces["ens37"].ReadOnly == "" {
		t.Fatal("missing source allowed file creation", err)
	}
	p.NewInterfaceFile = "../escape.yaml"
	if p.Validate() == nil {
		t.Fatal("source selection accepted a path")
	}
}

func TestInlineSourcesPreserveUnknownValuesAndDoNotMutatePreview(t *testing.T) {
	for _, source := range []string{
		"# keep comment\nnetwork:\n  version: 2\n  renderer: networkd\n  ethernets:\n    ens19: {dhcp4: false, addresses: [192.0.2.10/24], optional: true, mtu: 1500}\n",
		"# keep comment\nnetwork: {version: 2, renderer: networkd, ethernets: {ens19: {dhcp4: false, addresses: [192.0.2.10/24], optional: true, mtu: 1500}}}\n",
	} {
		root, p := testTree(t)
		writeTest(t, root, "etc/netplan/arbitrary-name.yaml", source)
		snapshot := discoverTest(t, root, p)
		modified, err := Modify(snapshot.View, []Change{{Kind: "interface", Name: "ens19", Field: "addresses", Action: "set", Values: []string{"192.0.2.20/24"}}})
		if err != nil {
			t.Fatal(err)
		}
		changed, err := snapshot.Render(modified)
		if err != nil {
			t.Fatal(err)
		}
		data := changed["etc/netplan/arbitrary-name.yaml"]
		if !strings.Contains(string(data), "# keep comment") || !strings.Contains(string(data), "optional: true") || !strings.Contains(string(data), "mtu: 1500") || !strings.Contains(string(data), "192.0.2.20/24") {
			t.Fatal("normalization lost an unknown value or comment", string(data))
		}
		if string(snapshot.Files[0].Data) != source || snapshot.fragments["ens19"][0].node.Style&yaml.FlowStyle == 0 && strings.Contains(source, "ens19: {") && strings.Contains(source, "network:\n") {
			t.Fatal("render changed its baseline snapshot")
		}
		if data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml")); string(data) != source {
			t.Fatal("preview wrote persistent YAML")
		}
	}
}
