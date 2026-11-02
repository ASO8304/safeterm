package networkmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnknownInterfaceNeedsExplicitPolicyAndCreatesMACMatchedSource(t *testing.T) {
	root, p := testTree(t)
	names := map[string]string{"ens19": "00:11:22:33:44:55", "ens20": "00:11:22:33:44:66"}
	view, err := Discover(root, p, names)
	if err != nil || view.View.Interfaces["ens20"].ReadOnly == "" || view.View.MACs["ens20"] != names["ens20"] {
		t.Fatal("unlisted hardware was editable or MAC missing", view, err)
	}
	p.AllowAllInterfaces = true
	snapshot, err := Discover(root, p, names)
	if err != nil || snapshot.View.Interfaces["ens20"].ReadOnly != "" {
		t.Fatal("opt-in did not allow new NIC", snapshot, err)
	}
	modified, err := Modify(snapshot.View, []Change{{Kind: "interface", Name: "ens20", Action: "set", Field: "dhcp4", Values: []string{"enable"}}})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := snapshot.Render(modified)
	if err != nil || !strings.Contains(string(changed[managedInterfaces]), "macaddress: 00:11:22:33:44:66") || !strings.Contains(string(changed[managedInterfaces]), "dhcp4: true") {
		t.Fatal("new definition was not MAC matched", changed, err)
	}
	if _, err := os.Stat(filepath.Join(root, managedInterfaces)); !os.IsNotExist(err) {
		t.Fatal("preview wrote a persistent source", err)
	}
	p.Files = []string{"arbitrary-name.yaml"}
	blocked, err := Discover(root, p, names)
	if err != nil || !strings.Contains(blocked.View.Interfaces["ens20"].ReadOnly, "source filename") {
		t.Fatal("wildcard bypassed file policy", blocked, err)
	}
}

func TestNewInterfaceTrialPersistsOnlyOnConfirmAndRollbackRemovesOnlyCreatedFile(t *testing.T) {
	root, p := testTree(t)
	p.AllowAllInterfaces = true
	backend := &fakeBackend{}
	m, err := New(Options{Root: root, State: filepath.Join(root, "state"), Policy: func() (Policy, error) { return p, nil }, Interfaces: func() map[string]string {
		return map[string]string{"ens19": "00:11:22:33:44:55", "ens20": "00:11:22:33:44:66"}
	}, Backend: backend})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	before, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	read := operatorCall(t, m, Request{Action: "read"})
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens20", Action: "set", Field: "dhcp4", Values: []string{"enable"}}}})
	operatorCall(t, m, Request{Action: "apply"})
	if _, err := os.Stat(filepath.Join(root, managedInterfaces)); !os.IsNotExist(err) {
		t.Fatal("trial wrote persistent new-interface file", err)
	}
	operatorCall(t, m, Request{Action: "confirm"})
	if data, err := os.ReadFile(filepath.Join(root, managedInterfaces)); err != nil || !strings.Contains(string(data), "safeterm-001122334466:") {
		t.Fatal("confirmed definition missing", err, string(data))
	}
	operatorCall(t, m, Request{Action: "rollback"})
	if _, err := os.Stat(filepath.Join(root, managedInterfaces)); !os.IsNotExist(err) {
		t.Fatal("rollback left a newly created persistent definition", err)
	}
	after, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if !bytes.Equal(before, after) || backend.restored != 1 {
		t.Fatal("rollback changed another source or did not restore networking")
	}
}

func TestPersistentRenameReusesMACRuleAndDoesNotApplyLiveNetworking(t *testing.T) {
	m, root, p, backend := managerTest(t)
	p.Interfaces = append(p.Interfaces, "lan0")
	m.opts.Policy = func() (Policy, error) { return p, nil }
	m.opts.Interfaces = func() map[string]string { return map[string]string{"ens19": "00:0c:29:ef:e4:51"} }
	if err := prepareNamingDirectory(root); err != nil {
		t.Fatal(err)
	}
	rulePath := "etc/systemd/network/10-persistent-net.link"
	before := "# Administrator rule\r\n[Match]\r\nMACAddress=00:0c:29:ef:e4:51\r\n\r\n[Link]\r\n# Retain MTU\r\nMTUBytes=1500\r\nName=ens19\r\n"
	writeTest(t, root, rulePath, before)
	if err := os.Chmod(filepath.Join(root, rulePath), 0640); err != nil {
		t.Fatal(err)
	}
	result := operatorCall(t, m, Request{Action: "rename", Interface: "ens19", NewName: "lan0"})
	data, err := os.ReadFile(filepath.Join(root, rulePath))
	if err != nil || !strings.Contains(string(data), "Name=lan0\r\n") || !strings.Contains(string(data), "# Retain MTU\r\nMTUBytes=1500\r\n") || !strings.Contains(result.Text, "live interface was not renamed") {
		t.Fatal(string(data), result, err)
	}
	info, _ := os.Stat(filepath.Join(root, rulePath))
	files, _ := os.ReadDir(filepath.Join(root, "etc/systemd/network"))
	if info.Mode().Perm() != 0640 || len(files) != 1 || backend.trial != nil || backend.restored != 0 {
		t.Fatal("rename changed metadata, created a duplicate rule or applied networking")
	}
	config, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if !strings.Contains(string(config), "macaddress: 00:0c:29:ef:e4:51") || !strings.Contains(string(config), "addresses: [192.0.2.10/24]") || !strings.Contains(string(config), "# important") {
		t.Fatal("Netplan was not safely bound to the NIC", string(config))
	}
	view := operatorCall(t, m, Request{Action: "read"})
	if view.View.Names["ens19"] != "lan0" || view.View.MACs["ens19"] != "00:0c:29:ef:e4:51" {
		t.Fatal("pending naming metadata missing", view)
	}
	m.opts.Interfaces = func() map[string]string { return map[string]string{"lan0": "00:0c:29:ef:e4:51"} }
	view = operatorCall(t, m, Request{Action: "read"})
	if view.View.Interfaces["lan0"].ReadOnly != "" || view.View.Interfaces["lan0"].ID != "ens19" || len(view.View.Interfaces["lan0"].Addresses) != 1 {
		t.Fatal("renamed NIC lost its configuration match", view)
	}
}

func TestRenameCreatesRuleOnlyWhenAbsentAndRejectsAmbiguousOrUntrustedRules(t *testing.T) {
	for _, condition := range []string{"new", "duplicate", "shared", "symlink", "unsafe-name", "target-not-allowed", "read-only"} {
		t.Run(condition, func(t *testing.T) {
			m, root, p, backend := managerTest(t)
			p.AllowAllInterfaces = true
			m.opts.Policy = func() (Policy, error) { return p, nil }
			m.opts.Interfaces = func() map[string]string { return map[string]string{"ens19": "00:11:22:33:44:55"} }
			if err := prepareNamingDirectory(root); err != nil {
				t.Fatal(err)
			}
			name := "lan0"
			rule := "[Match]\nMACAddress=00:11:22:33:44:55\n[Link]\nName=ens19\n"
			switch condition {
			case "duplicate":
				writeTest(t, root, "etc/systemd/network/10-a.link", rule)
				writeTest(t, root, "etc/systemd/network/20-b.link", rule)
			case "shared":
				writeTest(t, root, "etc/systemd/network/10-a.link", strings.Replace(rule, "MACAddress=00:11:22:33:44:55", "MACAddress=00:11:22:33:44:55 00:11:22:33:44:66", 1))
			case "symlink":
				if err := os.Symlink(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"), filepath.Join(root, "etc/systemd/network/10-a.link")); err != nil {
					t.Fatal(err)
				}
			case "unsafe-name":
				name = "../outside"
			case "target-not-allowed":
				p.AllowAllInterfaces = false
			case "read-only":
				p.Interfaces, p.AllowAllInterfaces = nil, false
			}
			result, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "rename", Interface: "ens19", NewName: name})
			if condition == "new" {
				path := filepath.Join(root, "etc/systemd/network/10-001122334455-persistent-net.link")
				if err != nil || !strings.Contains(result.Text, "next reboot") {
					t.Fatal(result, err)
				}
				if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "Name=lan0") {
					t.Fatal(string(data), err)
				}
			} else if err == nil {
				t.Fatal("unsafe or ambiguous rename accepted", condition)
			}
			if backend.trial != nil || backend.restored != 0 {
				t.Fatal("rename applied live networking")
			}
		})
	}
}

func TestNamingRecoveryFinishesFullyWrittenRuleWithoutRunningNetplan(t *testing.T) {
	m, root, _, backend := managerTest(t)
	if err := prepareNamingDirectory(root); err != nil {
		t.Fatal(err)
	}
	path := "etc/systemd/network/10-persistent-net.link"
	writeTest(t, root, path, "[Match]\nMACAddress=00:11:22:33:44:55\n[Link]\nName=ens19\n")
	old, err := readTrusted(root, path)
	if err != nil {
		t.Fatal(err)
	}
	changed := renameLink(old, "00:11:22:33:44:55", "lan0")
	tx := namingTransaction{Files: []File{old}, Changed: map[string][]byte{path: changed}}
	if err := m.save("naming-pending.json", tx); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(root, old, changed); err != nil {
		t.Fatal(err)
	}
	if err := m.recoverNaming(); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(filepath.Join(root, path))
	if !bytes.Equal(changed, restored) || backend.restored != 0 {
		t.Fatal("recovery failed or ran live networking")
	}
	if _, err := callTest(m, "viewer", []string{"network-viewer"}, Request{Action: "rename", Interface: "ens19", NewName: "lan0"}); err == nil {
		t.Fatal("read-only role renamed a NIC")
	}
}

func TestPartialNamingRecoveryRestoresSourcesAndLeavesOriginalLink(t *testing.T) {
	m, root, _, backend := managerTest(t)
	if err := prepareNamingDirectory(root); err != nil {
		t.Fatal(err)
	}
	rulePath := "etc/systemd/network/10-persistent-net.link"
	writeTest(t, root, rulePath, "[Match]\nMACAddress=00:11:22:33:44:55\n[Link]\nName=ens19\n")
	link, err := readTrusted(root, rulePath)
	if err != nil {
		t.Fatal(err)
	}
	source, err := readTrusted(root, "etc/netplan/arbitrary-name.yaml")
	if err != nil {
		t.Fatal(err)
	}
	next := []byte(strings.Replace(string(source.Data), "192.0.2.10/24", "192.0.2.20/24", 1))
	tx := namingTransaction{Files: []File{source, link}, Changed: map[string][]byte{source.Path: next, link.Path: renameLink(link, "00:11:22:33:44:55", "lan0")}}
	if err := m.save("naming-pending.json", tx); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(root, source, next); err != nil {
		t.Fatal(err)
	}
	if err := m.recoverNaming(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, source.Path))
	unchanged, _ := os.ReadFile(filepath.Join(root, link.Path))
	if !bytes.Equal(data, source.Data) || !bytes.Equal(unchanged, link.Data) || backend.restored != 0 {
		t.Fatal("partial recovery did not restore exactly the original files")
	}
}

func TestNamingSharedMACDropInsAndMetadataDoNotPermitSilentReplacement(t *testing.T) {
	root, _ := testTree(t)
	if err := prepareNamingDirectory(root); err != nil {
		t.Fatal(err)
	}
	path := "etc/systemd/network/10-persistent-net.link"
	data := "[Match]\nMACAddress=\"00:11:22:33:44:55\"\n[Link]\nName=\"ens19\"\n"
	writeTest(t, root, path, data)
	rules, err := readLinkRules(root)
	if err != nil {
		t.Fatal(err)
	}
	file, err := selectLinkRule(root, "00:11:22:33:44:55", rules)
	if err != nil || file.Path != path {
		t.Fatal("quoted MAC/name was not recognized", file, err)
	}
	if err := os.MkdirAll(filepath.Join(root, path+".d"), 0755); err != nil {
		t.Fatal(err)
	}
	writeTest(t, root, path+".d/99-name.conf", "[Link]\nName=other\n")
	if _, err := selectLinkRule(root, "00:11:22:33:44:55", rules); err == nil {
		t.Fatal("drop-in could silently override the new name")
	}
}
