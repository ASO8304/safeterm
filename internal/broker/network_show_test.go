package broker

import (
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/networkmanager"
)

func TestInterfaceShowIsScopedSortedAndReadable(t *testing.T) {
	s, _, out := editorFixture(t)
	api := &networkFixtureAPI{view: networkmanager.View{Version: "internal-version-hash", States: map[string]string{"ip0": "up", "ip1": "down"}, Owner: "private-owner", Interfaces: map[string]networkmanager.Interface{
		"ip1": {Name: "ip1", DHCP4: true},
		"ip0": {Name: "ip0", ID: "ip0", Addresses: []string{"192.168.222.10/24", "2001:db8::10/64"}, DNS: []string{"9.9.9.9"}, Files: []string{"etc/netplan/private-source.yaml"}, ReadOnly: "interface is outside administrator policy; a Linux administrator must add ip0"},
	}, Routes: map[string]networkmanager.Route{"private-route": {Name: "private-route", Interface: "ip0", Gateway: "203.0.113.254"}}}}
	s.SetNetworkClient(api)
	editorCommands(t, s, "config network interface")
	requests := len(api.requests)
	out.Reset()
	editorCommands(t, s, "show")
	text := out.String()
	for _, want := range []string{"Interfaces (configuration draft)", "NAME", "STATE", "up", "down", "DHCP4", "ADDRESSES", "EDITING", "ip0", "ip1", "192.168.222.10/24", "2001:db8::10/64", "Read-only", "Allowed", "Not allowed by administrator policy.", networkmanager.PolicyPath, "No unsaved interface edits.", "edit NAME"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if strings.Index(text, "ip0") > strings.Index(text, "ip1") {
		t.Fatal("interfaces not sorted")
	}
	for _, hidden := range []string{"internal-version-hash", "private-owner", "private-source.yaml", "private-route", "203.0.113.254", "9.9.9.9", "\"dhcp4\"", "\"pending\"", "null", "{", "}"} {
		if strings.Contains(text, hidden) {
			t.Fatalf("summary contains unrelated/internal data %q: %s", hidden, text)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if len(strings.TrimSuffix(line, "\r")) > 80 {
			t.Fatalf("normal summary exceeds 80 columns: %q", line)
		}
	}
	if len(api.requests) != requests || s.networkEdit.item != "" || len(s.networkEdit.changes) != 0 {
		t.Fatal("show changed editor/backend")
	}
	t.Log("\n" + text)
}

func TestSelectedInterfaceShowIncludesOnlyItsDetailsAndStagedValues(t *testing.T) {
	s, _, out := editorFixture(t)
	mtu := 1500
	api := &networkFixtureAPI{view: networkmanager.View{Version: "internal-version-hash", Interfaces: map[string]networkmanager.Interface{
		"ip0": {Name: "ip0", ID: "uplink", Addresses: []string{"192.168.222.10/24"}, DNS: []string{"1.1.1.1", "9.9.9.9"}, SearchDomains: []string{"example.test"}, MTU: &mtu, Files: []string{"etc/netplan/uplink.yaml"}},
		"ip1": {Name: "ip1", Addresses: []string{"198.51.100.10/24"}},
	}, Routes: map[string]networkmanager.Route{}}}
	s.SetNetworkClient(api)
	editorCommands(t, s, "config network interface", "edit ip0", "append addresses 192.168.222.100/24", "set mtu 1400")
	requests := len(api.requests)
	beforeChanges := append([]networkmanager.Change{}, s.networkEdit.changes...)
	out.Reset()
	editorCommands(t, s, "show")
	for _, want := range []string{"Interface: ip0 (Editing Allowed)", "Editing", "Allowed", "DHCP4", "off", "192.168.222.10/24", "192.168.222.100/24", "DNS Servers", "1.1.1.1", "9.9.9.9", "Search Domains", "example.test", "MTU", "1400", "Netplan ID", "uplink", "/etc/netplan/uplink.yaml", "* Unsaved edits staged"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	for _, hidden := range []string{"internal-version-hash", "198.51.100.10/24", "ip1", "1500", "null", "\"addresses\""} {
		if strings.Contains(out.String(), hidden) {
			t.Fatalf("selected detail contains %q: %s", hidden, out)
		}
	}
	if len(api.requests) != requests || !reflect.DeepEqual(beforeChanges, s.networkEdit.changes) || len(api.view.Interfaces["ip0"].Addresses) != 1 || *api.view.Interfaces["ip0"].MTU != 1500 {
		t.Fatal("show saved or changed the draft")
	}
	editorCommands(t, s, "next")
	out.Reset()
	editorCommands(t, s, "show")
	if !strings.Contains(out.String(), "* Unsaved edits") || !strings.Contains(out.String(), "ip0 *") || !strings.Contains(out.String(), "ip1") {
		t.Fatal("summary did not mark/show staged interfaces")
	}
}

func TestInterfaceShowExplainsEmptyValuesAndReadOnlyReasons(t *testing.T) {
	for _, reason := range []string{"", "legacy gateway4/gateway6 must be migrated to routes by the administrator", "physical interface is not present", "source mode=0666 permits group/world write"} {
		t.Run(reason, func(t *testing.T) {
			s, _, out := editorFixture(t)
			api := &networkFixtureAPI{view: networkmanager.View{Interfaces: map[string]networkmanager.Interface{"ip0": {Name: "ip0", ID: "ip0", ReadOnly: reason}}, Routes: map[string]networkmanager.Route{}}}
			s.SetNetworkClient(api)
			editorCommands(t, s, "config network interface", "edit ip0")
			out.Reset()
			editorCommands(t, s, "show")
			if !strings.Contains(out.String(), "System default") || !strings.Contains(out.String(), "Automatic") || strings.Contains(out.String(), "null") {
				t.Fatal(out.String())
			}
			if reason != "" && (!strings.Contains(out.String(), "Read-only") || !strings.Contains(out.String(), "Read-only:")) {
				t.Fatal("read-only reason hidden", out)
			}
			if reason != "" && strings.Contains(out.String(), "set FIELD VALUE") {
				t.Fatal("read-only details suggest editing", out)
			}
			if strings.Contains(reason, "gateway4") && !strings.Contains(out.String(), "default route") {
				t.Fatal("gateway guidance hidden", out)
			}
			if strings.Contains(reason, "0666") && !strings.Contains(out.String(), "0666") {
				t.Fatal("specific unknown reason was lost", out)
			}
			if len(api.requests) != 1 {
				t.Fatal("display contacted manager")
			}
		})
	}
	s, _, out := editorFixture(t)
	s.SetNetworkClient(&networkFixtureAPI{view: networkmanager.View{}})
	editorCommands(t, s, "config network interface")
	out.Reset()
	editorCommands(t, s, "show")
	if !strings.Contains(out.String(), "No supported interfaces found") || !strings.Contains(out.String(), "back") {
		t.Fatal(out.String())
	}
}
