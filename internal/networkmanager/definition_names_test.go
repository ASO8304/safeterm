package networkmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const replacementSource = "# administrator source\nnetwork:\n  version: 2\n  renderer: networkd\n  ethernets:\n    ip0:\n      dhcp4: false\n      addresses: [192.168.222.10/24]\n      routes:\n        - to: 0.0.0.0/0\n          via: 192.168.222.1\n    safeterm-000c29efe45b:\n      dhcp4: false\n      addresses: [192.168.222.11/24]\n      match:\n        macaddress: 00:0c:29:ef:e4:5b\n      optional: true # keep exactly\n      routes:\n        - to: 0.0.0.0/0\n          via: 192.168.222.1\n          metric: 1\n"

func replacementManager(t *testing.T) (*Manager, string) {
	t.Helper()
	m, root, _ := namingManager(t)
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", replacementSource)
	m.opts.Interfaces = func() map[string]string {
		return map[string]string{"ens37": "00:0c:29:ef:e4:5b"}
	}
	return m, root
}

func TestReplacementNICUsesPersistentIDAndExplicitCleanup(t *testing.T) {
	m, root := replacementManager(t)
	read := operatorCall(t, m, Request{Action: "read"})
	if len(read.View.Interfaces) != 1 || len(read.View.AbsentInterfaces) != 1 || len(read.View.Routes) != 1 {
		t.Fatal("absent configuration mixed into present devices", read.View)
	}
	changes := []Change{{Kind: "interface", Name: "ens37", Action: "set", Field: "name", Values: []string{"ip1"}}}
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: changes})
	operatorCall(t, m, Request{Action: "apply"})
	operatorCall(t, m, Request{Action: "confirm"})
	path := filepath.Join(root, "etc/netplan/arbitrary-name.yaml")
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "    ip1:\n") || strings.Contains(string(data), "safeterm-") || !strings.Contains(string(data), "    ip0:\n") || !strings.Contains(string(data), "optional: true # keep exactly") {
		t.Fatal("rename did not preserve old definition or readable ID", string(data))
	}
	read = operatorCall(t, m, Request{Action: "read"})
	if read.View.Interfaces["ens37"].ID != "ip1" || read.View.Interfaces["ens37"].PersistentName != "ip1" {
		t.Fatal("renamed definition no longer matches current hardware", read.View)
	}
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ip0", Action: "delete"}}})
	diff := operatorCall(t, m, Request{Action: "diff"})
	if !strings.Contains(diff.Text, "absent interface ip0") {
		t.Fatal("cleanup missing from review", diff)
	}
	// Reload the saved candidate: an empty absent inventory must survive JSON.
	read = operatorCall(t, m, Request{Action: "read"})
	if len(read.View.AbsentInterfaces) != 0 {
		t.Fatal("deletion lost on reopening candidate")
	}
	operatorCall(t, m, Request{Action: "apply"})
	if trial, _ := os.ReadFile(path); string(trial) != string(data) {
		t.Fatal("cleanup trial changed persistent source")
	}
	operatorCall(t, m, Request{Action: "confirm"})
	cleaned, _ := os.ReadFile(path)
	if strings.Contains(string(cleaned), "    ip0:") || strings.Contains(string(cleaned), "192.168.222.10/24") || !strings.Contains(string(cleaned), "metric: 1") {
		t.Fatal("cleanup retained stale configuration or changed present NIC", string(cleaned))
	}
	operatorCall(t, m, Request{Action: "rollback"})
	if restored, _ := os.ReadFile(path); string(restored) != string(data) {
		t.Fatal("rollback did not restore exact pre-cleanup source")
	}
}

func TestRetainedIDMustBeExplicitlyDeletedBeforeReuse(t *testing.T) {
	m, root := replacementManager(t)
	read := operatorCall(t, m, Request{Action: "read"})
	changes := []Change{{Kind: "interface", Name: "ens37", Action: "set", Field: "name", Values: []string{"ip0"}}}
	_, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "save", Version: read.View.Version, Changes: changes})
	if err == nil || !strings.Contains(err.Error(), "explicitly delete") {
		t.Fatal("rename silently overwrote retained configuration", err)
	}
	changes = append([]Change{{Kind: "interface", Name: "ip0", Action: "delete"}}, changes...)
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: changes})
	operatorCall(t, m, Request{Action: "apply"})
	operatorCall(t, m, Request{Action: "confirm"})
	data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if strings.Count(string(data), "    ip0:\n") != 1 || strings.Contains(string(data), "192.168.222.10/24") || !strings.Contains(string(data), "192.168.222.11/24") {
		t.Fatal("replacement definition corrupt", string(data))
	}
	// Simulate the next activation: the MAC-matched configuration still resolves.
	m.opts.Interfaces = func() map[string]string { return map[string]string{"ip0": "00:0c:29:ef:e4:5b"} }
	read = operatorCall(t, m, Request{Action: "read"})
	if len(read.View.Interfaces) != 1 || read.View.Interfaces["ip0"].ID != "ip0" || len(read.View.AbsentInterfaces) != 0 {
		t.Fatal("name activation resurrected stale source", read.View)
	}
}

func TestExistingPersistentNameRepairsLegacyIDDuringEdit(t *testing.T) {
	m, root := replacementManager(t)
	if err := prepareNamingDirectory(root); err != nil {
		t.Fatal(err)
	}
	writeTest(t, root, "etc/systemd/network/10-persistent-net.link", "[Match]\nMACAddress=00:0c:29:ef:e4:5b\n[Link]\nName=ip1\n")
	read := operatorCall(t, m, Request{Action: "read"})
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens37", Field: "mtu", Action: "set", Values: []string{"1400"}}}})
	operatorCall(t, m, Request{Action: "apply"})
	operatorCall(t, m, Request{Action: "confirm"})
	data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if strings.Contains(string(data), "safeterm-") || !strings.Contains(string(data), "    ip1:\n") {
		t.Fatal("existing .link name did not repair old ID", string(data))
	}
}

func TestReassertExistingNameBindsLegacyNameOnlyDefinition(t *testing.T) {
	m, root, _ := namingManager(t)
	if err := prepareNamingDirectory(root); err != nil {
		t.Fatal(err)
	}
	writeTest(t, root, "etc/systemd/network/10-persistent-net.link", "[Match]\nMACAddress=00:11:22:33:44:55\n[Link]\nName=lan0\n")
	// Repeating the already-persisted name must repair its Netplan key without
	// needing a gratuitous address or MTU change. Preserve this intent on disk.
	saveName(t, m, "lan0")
	if diff := operatorCall(t, m, Request{Action: "diff"}); !strings.Contains(diff.Text, "Netplan ID: ens19 -> lan0") {
		t.Fatal("saved name repair missing from diff", diff)
	}
	read := operatorCall(t, m, Request{Action: "read"})
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version})
	operatorCall(t, m, Request{Action: "apply"})
	operatorCall(t, m, Request{Action: "confirm"})
	data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if !strings.Contains(string(data), "    lan0:") || !strings.Contains(string(data), "00:11:22:33:44:55") {
		t.Fatal("reasserted name lost its ID update or live MAC match", string(data))
	}
	read = operatorCall(t, m, Request{Action: "read"})
	if len(read.View.Interfaces["ens19"].Addresses) != 1 {
		t.Fatal("renamed key stopped matching the live device", read.View)
	}
}

func TestAbsentDeletionRefusesPresentPolicyDeniedAndReferencedDefinitions(t *testing.T) {
	for _, mode := range []string{"present", "policy", "file-policy", "reference", "advanced-match", "empty-match", "pending-set-name"} {
		t.Run(mode, func(t *testing.T) {
			m, root := replacementManager(t)
			p := DefaultPolicy()
			p.AllowAllInterfaces = true
			source := replacementSource
			switch mode {
			case "present":
				m.opts.Interfaces = func() map[string]string {
					return map[string]string{"ip0": "00:11:22:33:44:55", "ens37": "00:0c:29:ef:e4:5b"}
				}
			case "policy":
				p.AllowAllInterfaces, p.Interfaces = false, []string{"ens37"}
			case "file-policy":
				p.Files = []string{"other.yaml"}
			case "reference":
				source += "  bridges:\n    br0:\n      interfaces: [ip0]\n"
			case "advanced-match":
				source = strings.Replace(source, "    ip0:\n", "    ip0:\n      match: {driver: absent-driver}\n", 1)
			case "empty-match":
				source = strings.Replace(source, "    ip0:\n", "    ip0:\n      match: {}\n", 1)
				m.opts.Interfaces = func() map[string]string {
					return map[string]string{"ens37": "00:0c:29:ef:e4:5b", "ens38": "00:11:22:33:44:55"}
				}
			case "pending-set-name":
				source = strings.Replace(source, "    ip0:\n", "    ip0:\n      match: {macaddress: '00:0c:29:ef:e4:5b'}\n      set-name: absent0\n", 1)
			}
			m.opts.Policy = func() (Policy, error) { return p, nil }
			writeTest(t, root, "etc/netplan/arbitrary-name.yaml", source)
			read := operatorCall(t, m, Request{Action: "read"})
			if _, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ip0", Action: "delete"}}}); err == nil {
				t.Fatal("unsafe cleanup accepted")
			}
			if data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml")); string(data) != source {
				t.Fatal("rejected cleanup wrote source")
			}
		})
	}
}

func TestDefinitionRenameUpdatesAllFragmentsAndRejectsReferencesOrVirtualID(t *testing.T) {
	for _, mode := range []string{"fragments", "reference", "virtual-collision"} {
		t.Run(mode, func(t *testing.T) {
			m, root, _ := namingManager(t)
			source := "network:\n  version: 2\n  ethernets:\n    ens19:\n      mtu: 1500\n"
			if mode == "reference" {
				source += "  vlans:\n    vlan10:\n      id: 10\n      link: ens19\n"
			}
			if mode == "virtual-collision" {
				source += "  bridges:\n    lan0: {}\n"
			}
			writeTest(t, root, "etc/netplan/99-extra.yaml", source)
			read := operatorCall(t, m, Request{Action: "read"})
			_, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens19", Action: "set", Field: "name", Values: []string{"lan0"}}}})
			if mode != "fragments" {
				if err == nil {
					t.Fatal("unsafe ID rename accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			operatorCall(t, m, Request{Action: "apply"})
			operatorCall(t, m, Request{Action: "confirm"})
			for _, base := range []string{"arbitrary-name.yaml", "99-extra.yaml"} {
				data, _ := os.ReadFile(filepath.Join(root, "etc/netplan", base))
				if strings.Contains(string(data), "    ens19:") || !strings.Contains(string(data), "    lan0:") {
					t.Fatal("source fragment kept the old ID", string(data))
				}
			}
		})
	}
}

func TestDeleteLastAbsentDefinitionInlineAndLegacyCandidate(t *testing.T) {
	m, root := replacementManager(t)
	source := "# Retain this comment\nnetwork: {version: 2, ethernets: {ip0: {dhcp4: false}}}\n"
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", source)
	m.opts.Interfaces = func() map[string]string { return map[string]string{} }
	read := operatorCall(t, m, Request{Action: "read"})
	legacy := candidate{Owner: "ssh:admin", Base: read.View.Version, Revision: "legacy", View: *read.View}
	legacy.View.AbsentInterfaces = nil
	if err := m.save(candidateFile("ssh:admin"), legacy); err != nil {
		t.Fatal(err)
	}
	read = operatorCall(t, m, Request{Action: "read"})
	if len(read.View.AbsentInterfaces) != 1 {
		t.Fatal("legacy candidate implied deletion")
	}
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ip0", Action: "delete"}}})
	operatorCall(t, m, Request{Action: "apply"})
	operatorCall(t, m, Request{Action: "confirm"})
	operatorCall(t, m, Request{Action: "read"})
	data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if !strings.Contains(string(data), "ethernets: {}") || !strings.Contains(string(data), "# Retain this comment") {
		t.Fatal("last-definition deletion produced invalid mapping or lost comment", string(data))
	}
}

func TestReplacementCanExplicitlyReuseItsCurrentNameWithoutGeneratedID(t *testing.T) {
	m, root, _ := namingManager(t)
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", "network:\n  version: 2\n  ethernets:\n    ens19:\n      match: {macaddress: '00:11:22:33:44:66'}\n      addresses: [192.0.2.99/24]\n")
	read := operatorCall(t, m, Request{Action: "read"})
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{
		{Kind: "interface", Name: "ens19", Action: "delete"},
		{Kind: "interface", Name: "ens19", Action: "set", Field: "addresses", Values: []string{"192.0.2.10/24"}},
	}})
	operatorCall(t, m, Request{Action: "apply"})
	operatorCall(t, m, Request{Action: "confirm"})
	data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if strings.Contains(string(data), "safeterm-") || strings.Contains(string(data), "192.0.2.99/24") || !strings.Contains(string(data), "    ens19:") {
		t.Fatal("private temporary ID or removed NIC settings persisted", string(data))
	}
	read = operatorCall(t, m, Request{Action: "read"})
	if read.View.Interfaces["ens19"].ID != "ens19" || len(read.View.AbsentInterfaces) != 0 {
		t.Fatal("replacement did not resolve after explicit cleanup", read.View)
	}
}

func TestCleanupCannotApplyAfterHardwareReturnsOrRollbackOutsidePolicy(t *testing.T) {
	for _, mode := range []string{"returned", "rollback-policy"} {
		t.Run(mode, func(t *testing.T) {
			m, root := replacementManager(t)
			read := operatorCall(t, m, Request{Action: "read"})
			operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ip0", Action: "delete"}}})
			if mode == "returned" {
				m.opts.Interfaces = func() map[string]string {
					return map[string]string{"ip0": "00:11:22:33:44:55", "ens37": "00:0c:29:ef:e4:5b"}
				}
				if _, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "apply"}); err == nil {
					t.Fatal("cleanup applied after supposedly absent hardware returned")
				}
				if data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml")); string(data) != replacementSource {
					t.Fatal("stale cleanup touched source")
				}
				return
			}
			operatorCall(t, m, Request{Action: "apply"})
			operatorCall(t, m, Request{Action: "confirm"})
			p := DefaultPolicy()
			p.Interfaces = []string{"ens37"}
			m.opts.Policy = func() (Policy, error) { return p, nil }
			if _, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "rollback"}); err == nil || !strings.Contains(err.Error(), "outside current policy") {
				t.Fatal("rollback restored a definition outside current interface policy", err)
			}
		})
	}
}
