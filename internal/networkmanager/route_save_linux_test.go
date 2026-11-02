package networkmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportedInvalidGatewayExplainsUnrelatedSaveFailureAndCanBeRepaired(t *testing.T) {
	m, root, _, backend := managerTest(t)
	bad := fixture + "      routes:\n        - to: default\n          via: 192.0.2.1/24\n"
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", bad)
	read := operatorCall(t, m, Request{Action: "read"})
	_, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens19", Action: "set", Field: "mtu", Values: []string{"1400"}}}})
	if err == nil || !strings.Contains(err.Error(), "default IPv4 route on interface ens19") || !strings.Contains(err.Error(), "set gateway 192.0.2.1") || strings.Contains(err.Error(), "route-") {
		t.Fatal("save blamed opaque internal route", err)
	}
	if _, err := os.Stat(filepath.Join(root, "state/candidate.json")); !os.IsNotExist(err) {
		t.Fatal("invalid candidate saved", err)
	}
	var name string
	for key := range read.View.Routes {
		name = key
	}
	operatorCall(t, m, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "route", Name: name, Action: "set", Field: "gateway", Values: []string{"192.0.2.1"}}, {Kind: "interface", Name: "ens19", Action: "set", Field: "mtu", Values: []string{"1400"}}}})
	data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
	if string(data) != bad || backend.trial != nil {
		t.Fatal("save changed active networking")
	}
	operatorCall(t, m, Request{Action: "apply"})
	operatorCall(t, m, Request{Action: "confirm"})
	view := operatorCall(t, m, Request{Action: "read"})
	if view.View.Routes[name].Gateway != "192.0.2.1" || view.View.Interfaces["ens19"].MTU == nil {
		t.Fatal("repair lost other staged settings", view)
	}
}
