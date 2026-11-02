package networkmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func namingManager(t *testing.T) (*Manager, string, *fakeBackend) {
	t.Helper()
	m, root, p, backend := managerTest(t)
	p.AllowAllInterfaces = true
	m.opts.Policy = func() (Policy, error) { return p, nil }
	m.opts.Interfaces = func() map[string]string { return map[string]string{"ens19": "00:11:22:33:44:55"} }
	return m, root, backend
}

func saveName(t *testing.T, m *Manager, future string, extra ...Change) {
	t.Helper()
	read := operatorCall(t, m, Request{Action: "read"})
	changes := append([]Change{{Kind: "interface", Name: "ens19", Field: "name", Action: "set", Values: []string{future}}}, extra...)
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: changes})
}

func TestNameCandidateDoesNotPersistBeforeConfirmAndRollsBackWithNetplan(t *testing.T) {
	m, root, backend := namingManager(t)
	before, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	link := filepath.Join(root, "etc/systemd/network/11-persistent-net.link")
	saveName(t, m, "lan0", Change{Kind: "interface", Name: "ens19", Field: "mtu", Action: "set", Values: []string{"1400"}})
	read := operatorCall(t, m, Request{Action: "read"})
	if read.View.Interfaces["ens19"].PersistentName != "lan0" {
		t.Fatal("saved candidate lost its future name")
	}
	for _, stage := range []string{"saved", "trial"} {
		if stage == "trial" {
			operatorCall(t, m, Request{Action: "apply"})
		}
		if data, err := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml")); err != nil || string(data) != string(before) {
			t.Fatal(stage, "changed persistent Netplan", err)
		}
		if _, err := os.Stat(link); !os.IsNotExist(err) {
			t.Fatal(stage, "created persistent naming rule", err)
		}
		if _, err := os.Stat(filepath.Join(root, "run/netplan/11-persistent-net.link")); !os.IsNotExist(err) {
			t.Fatal("link rule participated in the live trial", err)
		}
	}
	operatorCall(t, m, Request{Action: "confirm"})
	data, err := os.ReadFile(link)
	if err != nil || !strings.Contains(string(data), "Name=lan0") || !backend.trial.accepted {
		t.Fatal("confirmed name not persisted", string(data), err)
	}
	data, _ = os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if !strings.Contains(string(data), "mtu: 1400") || !strings.Contains(string(data), "macaddress:") || !strings.Contains(string(data), "00:11:22:33:44:55") {
		t.Fatal("combined name and IP settings were not committed together", string(data))
	}
	operatorCall(t, m, Request{Action: "rollback"})
	if _, err := os.Stat(link); !os.IsNotExist(err) {
		t.Fatal("rollback left a newly created name rule", err)
	}
	data, _ = os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if string(data) != string(before) || backend.restored != 1 {
		t.Fatal("rollback did not restore the original Netplan bytes")
	}
}

func TestNameCandidateDiscardAndTrialRollbackLeaveExistingRuleUntouched(t *testing.T) {
	for _, action := range []string{"discard", "rollback"} {
		t.Run(action, func(t *testing.T) {
			m, root, _ := namingManager(t)
			if err := prepareNamingDirectory(root); err != nil {
				t.Fatal(err)
			}
			path := "etc/systemd/network/10-persistent-net.link"
			original := "[Match]\nMACAddress=00:11:22:33:44:55\n[Link]\nName=ens19\n"
			writeTest(t, root, path, original)
			saveName(t, m, "lan0")
			if action == "rollback" {
				operatorCall(t, m, Request{Action: "apply"})
			}
			operatorCall(t, m, Request{Action: action})
			data, err := os.ReadFile(filepath.Join(root, path))
			if err != nil || string(data) != original {
				t.Fatal("unconfirmed name changed administrator rule", string(data), err)
			}
		})
	}
}

func TestNameConfirmRejectsNewDropInAndChangedRule(t *testing.T) {
	for _, change := range []string{"drop-in", "file", "vendor"} {
		t.Run(change, func(t *testing.T) {
			m, root, _ := namingManager(t)
			saveName(t, m, "lan0")
			operatorCall(t, m, Request{Action: "apply"})
			path := "etc/systemd/network/11-persistent-net.link"
			if change == "drop-in" {
				if err := os.MkdirAll(filepath.Join(root, path+".d"), 0755); err != nil {
					t.Fatal(err)
				}
				writeTest(t, root, path+".d/override.conf", "[Link]\nName=other\n")
			} else if change == "vendor" {
				path = "usr/lib/systemd/network/11-persistent-net.link"
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0755); err != nil {
					t.Fatal(err)
				}
				writeTest(t, root, path, "# new vendor rule\n")
			} else {
				writeTest(t, root, path, "# Linux administrator created this file\n")
			}
			if _, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "confirm"}); err == nil {
				t.Fatal("stale naming plan was persisted")
			}
		})
	}
}

func TestCompletedNamingCandidateRecoveryKeepsBootConsumedName(t *testing.T) {
	m, root, backend := namingManager(t)
	saveName(t, m, "lan0")
	operatorCall(t, m, Request{Action: "apply"})
	m.mu.Lock()
	tx := *m.pending
	tx.Phase = "committing"
	for _, file := range append(append([]File(nil), tx.Files...), tx.LinkFiles...) {
		if err := replaceFile(root, file, tx.changedData(file)); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.save("pending.json", &tx); err != nil {
		t.Fatal(err)
	}
	// Simulate a crashed process and a boot that already adopted the new name.
	m.pending, m.trial, m.closed = nil, nil, true
	m.mu.Unlock()
	_ = backend.trial.Finish(true)
	opts := m.opts
	opts.Interfaces = func() map[string]string { return map[string]string{"lan0": "00:11:22:33:44:55"} }
	recovered, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	view := operatorCall(t, recovered, Request{Action: "read"})
	if view.View.Interfaces["lan0"].PersistentName != "lan0" || backend.restored != 0 {
		t.Fatal("recovery undid a completed boot naming change", view)
	}
}

func TestMultipleStagedNamesGetDistinctRulesAndCannotShareTargetName(t *testing.T) {
	m, root, _ := namingManager(t)
	m.opts.Interfaces = func() map[string]string {
		return map[string]string{"ens19": "00:11:22:33:44:55", "ens20": "00:11:22:33:44:66"}
	}
	read := operatorCall(t, m, Request{Action: "read"})
	changes := []Change{
		{Kind: "interface", Name: "ens19", Field: "name", Action: "set", Values: []string{"lan0"}},
		{Kind: "interface", Name: "ens20", Field: "name", Action: "set", Values: []string{"lan0"}},
	}
	if _, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "save", Version: read.View.Version, Changes: changes}); err == nil {
		t.Fatal("two NICs can adopt the same future name")
	}
	changes[1].Values = []string{"lan1"}
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: changes})
	operatorCall(t, m, Request{Action: "apply"})
	operatorCall(t, m, Request{Action: "confirm"})
	for file, value := range map[string]string{"11-persistent-net.link": "Name=lan0", "12-persistent-net.link": "Name=lan1"} {
		data, err := os.ReadFile(filepath.Join(root, "etc/systemd/network", file))
		if err != nil || !strings.Contains(string(data), value) {
			t.Fatal("multiple staged names collided", file, string(data), err)
		}
	}
}
