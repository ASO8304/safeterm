package broker

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/contract"
	"safeterm/internal/networkmanager"
)

type networkFixtureAPI struct {
	requests []networkmanager.Request
	view     networkmanager.View
}

func (n *networkFixtureAPI) Call(_ context.Context, actor contract.Metadata, r networkmanager.Request) (networkmanager.Reply, error) {
	n.requests = append(n.requests, r)
	if actor.UserID != "alice" || actor.BrokerType != "ssh" {
		panic("lost authenticated identity")
	}
	if r.Action == "save" {
		v, err := networkmanager.Modify(n.view, r.Changes)
		if err != nil {
			return networkmanager.Reply{}, err
		}
		if err = networkmanager.ValidateView(v); err != nil {
			return networkmanager.Reply{}, err
		}
		n.view = v
	}
	v := n.view
	return networkmanager.Reply{View: &v, Text: r.Action}, nil
}
func TestNetworkCandidateCLIAndCompletion(t *testing.T) {
	s, _, out := editorFixture(t)
	api := &networkFixtureAPI{view: networkmanager.View{Version: "base", Interfaces: map[string]networkmanager.Interface{"ens19": {Name: "ens19", ID: "ens19", Addresses: []string{"192.0.2.1/24"}}}, Routes: map[string]networkmanager.Route{}}}
	s.SetNetworkClient(api)
	for _, test := range []struct {
		before, prefix string
		want           []string
	}{{"execute net", "net", []string{"network"}}, {"config network ", "", []string{"interface", "route"}}, {"execute network c", "c", []string{"confirm"}}} {
		if got := s.completionCandidates(context.Background(), test.before, test.prefix); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s: %v", test.before, got)
		}
	}
	editorCommands(t, s, "config network interface", "edit ens19", "set dhcp4 disable", "set addresses 192.0.2.10/24", "append addresses 192.0.2.11/24", "set dns 1.1.1.1", "set mtu 1400", "show", "diff")
	if got := s.completionCandidates(context.Background(), "set dhcp4 ", ""); !reflect.DeepEqual(got, []string{"disable", "enable"}) {
		t.Fatal(got)
	}
	if len(api.requests) != 1 {
		t.Fatal("local edits mutated backend")
	}
	if !strings.Contains(s.prompt(), "network/interface/ens19") {
		t.Fatal(s.prompt())
	}
	editorCommands(t, s, "next", "save")
	if api.requests[len(api.requests)-1].Action != "save" || len(api.view.Interfaces["ens19"].Addresses) != 2 {
		t.Fatal(api.requests)
	}
	editorCommands(t, s, "config network route", "edit office", "set interface ens19", "set destination 198.51.100.0/24", "set gateway 192.0.2.254", "set metric 100", "save")
	if api.view.Routes["office"].Gateway != "192.0.2.254" {
		t.Fatal(api.view.Routes)
	}
	before := len(api.requests)
	editorCommands(t, s, "execute network ?")
	if len(api.requests) != before {
		t.Fatal("help invoked backend")
	}
	editorCommands(t, s, "execute network apply", "execute network confirm", "execute network status")
	if !strings.Contains(out.String(), "Candidate saved") {
		t.Fatal(out.String())
	}
}
func TestNetworkAbortNeverSaves(t *testing.T) {
	s, _, _ := editorFixture(t)
	api := &networkFixtureAPI{view: networkmanager.View{Version: "base", Interfaces: map[string]networkmanager.Interface{"ens19": {Name: "ens19"}}, Routes: map[string]networkmanager.Route{}}}
	s.SetNetworkClient(api)
	editorCommands(t, s, "config network interface", "edit ens19", "set mtu 1450", "cancel")
	if len(api.requests) != 1 || s.networkEdit != nil {
		t.Fatal("abort saved candidate")
	}
}

func TestSystemInfoSubcommandsPreserveSignedOperationIDs(t *testing.T) {
	cfg := brokerTestConfig()
	for _, name := range []string{"memory_info", "disk_usage"} {
		op := cfg.Operations[0]
		op.Name = name
		cfg.Operations = append(cfg.Operations, op)
		cfg.Policies[0].Operations = append(cfg.Policies[0].Operations, name)
	}
	catalog, err := NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGateway{}
	s, _ := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader(""), &bytes.Buffer{})
	if got := s.completionCandidates(context.Background(), "execute system_info ", ""); !reflect.DeepEqual(got, []string{"disk", "host", "memory"}) {
		t.Fatal(got)
	}
	for _, tc := range []struct{ leaf, id string }{{"disk", "disk_usage"}, {"memory", "memory_info"}, {"host", "system_info"}} {
		editorCommands(t, s, "execute system_info "+tc.leaf)
		if got := client.requests[len(client.requests)-1].Command; got != tc.id {
			t.Fatalf("%s resolved to %s", tc.leaf, got)
		}
	}
}

func TestNetworkQuestionHelpDescribesContextsWithoutRequestsOrEdits(t *testing.T) {
	s, _, out := editorFixture(t)
	api := &networkFixtureAPI{view: networkmanager.View{Version: "base", Interfaces: map[string]networkmanager.Interface{"ens19": {Name: "ens19", ID: "ens19"}, "ens20": {Name: "ens20", ID: "ens20", ReadOnly: "interface is outside administrator policy"}}, Routes: map[string]networkmanager.Route{}}}
	s.SetNetworkClient(api)
	for _, tc := range []struct{ line, want string }{
		{"execute network ?", "Apply the saved candidate as a timed trial"},
		{"execute network apply ?", "confirmation is required"},
		{"execute network confirm ?", "persist its Netplan changes"},
		{"config network interface ?", "Select an existing network interface"},
		{"config network route ?", "choose a name for a new route"},
	} {
		out.Reset()
		editorCommands(t, s, tc.line)
		if !strings.Contains(strings.Join(strings.Fields(out.String()), " "), strings.Join(strings.Fields(tc.want), " ")) {
			t.Fatalf("%s: %q", tc.line, out.String())
		}
		if s.networkEdit != nil || len(api.requests) != 0 {
			t.Fatalf("help entered an editor or called the manager: %s", tc.line)
		}
	}
	editorCommands(t, s, "config network interface", "edit ens19", "set mtu 1400")
	beforeView := s.networkEdit.view
	beforeChanges := len(s.networkEdit.changes)
	beforeRequests := len(api.requests)
	tabBefore := s.completionCandidates(context.Background(), "set ", "")
	for _, tc := range []struct{ line, want string }{
		{"?", "Add values to a list"},
		{"edit ?", "ens20                   Select network interface; read-only: interface is outside administrator policy"},
		{"set ?", "Replace all IP addresses"},
		{"set addresses ?", "<cidr>..."},
		{"set dhcp4 ?", "Enable IPv4 DHCP"},
		{"append ?", "DNS search domains"},
		{"unselect ?", "DNS server IP addresses"},
		{"unset mtu ?", "Clear this field in the editor"},
		{"save ?", "does not apply networking"},
		{"cancel ?", "discard unsaved editor changes"},
		{"next ?", "Move up (keeps unsaved edits staged)"},
	} {
		out.Reset()
		editorCommands(t, s, tc.line)
		if !strings.Contains(strings.Join(strings.Fields(out.String()), " "), strings.Join(strings.Fields(tc.want), " ")) {
			t.Fatalf("%s: %q", tc.line, out.String())
		}
	}
	if s.networkEdit == nil || s.networkEdit.item != "ens19" || len(s.networkEdit.changes) != beforeChanges || !reflect.DeepEqual(beforeView, s.networkEdit.view) || len(api.requests) != beforeRequests {
		t.Fatal("help changed editor state or contacted manager")
	}
	if got := s.completionCandidates(context.Background(), "set ", ""); !reflect.DeepEqual(got, tabBefore) {
		t.Fatalf("Tab changed: %v -> %v", tabBefore, got)
	}
	editorCommands(t, s, "cancel", "config network route", "edit office")
	beforeRequests = len(api.requests)
	for _, tc := range []struct{ line, want string }{
		{"?", "Remove a named route from the candidate"},
		{"set ?", "Destination network CIDR"},
		{"set destination ?", "<network/CIDR>"},
		{"set interface ?", "ens19                   Use this allowed outgoing interface"},
		{"edit ?", "Choose a new route name"},
	} {
		out.Reset()
		editorCommands(t, s, tc.line)
		if !strings.Contains(strings.Join(strings.Fields(out.String()), " "), strings.Join(strings.Fields(tc.want), " ")) {
			t.Fatalf("%s: %q", tc.line, out.String())
		}
		if tc.line == "set interface ?" && strings.Contains(out.String(), "ens20") {
			t.Fatal("help offered read-only interface for a route")
		}
	}
	if len(api.requests) != beforeRequests || len(s.networkEdit.changes) != 0 || s.networkEdit.item != "office" {
		t.Fatal("route help mutated state")
	}
}

func TestRouteShowGroupsOnlyCandidateRoutesByInterface(t *testing.T) {
	s, _, out := editorFixture(t)
	metric := uint32(100)
	api := &networkFixtureAPI{view: networkmanager.View{Version: "private-version", Interfaces: map[string]networkmanager.Interface{
		"ip0": {Name: "ip0", ID: "ip0", Addresses: []string{"192.168.222.10/24"}, DNS: []string{"9.9.9.9"}, Files: []string{"etc/netplan/private-file.yaml"}},
		"ip1": {Name: "ip1", ID: "ip1", DHCP4: true},
	}, Routes: map[string]networkmanager.Route{
		"office":         {Name: "office", Interface: "ip0", Destination: "10.50.0.0/16", Gateway: "192.168.222.1", Metric: &metric},
		"default-uplink": {Name: "default-uplink", Interface: "ip1", Destination: "0.0.0.0/0", Gateway: "192.168.89.1"},
	}}}
	s.SetNetworkClient(api)
	editorCommands(t, s, "config network route")
	requests := len(api.requests)
	out.Reset()
	editorCommands(t, s, "show")
	text := out.String()
	for _, want := range []string{"Configured routes (candidate)", "Interface ip0", "Interface ip1", "NAME", "DESTINATION", "GATEWAY", "METRIC", "office", "10.50.0.0/16", "192.168.222.1", "100", "default-uplink", "0.0.0.0/0", "auto"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if strings.Index(text, "Interface ip0") > strings.Index(text, "Interface ip1") {
		t.Fatal("interface groups are not sorted")
	}
	for _, hidden := range []string{"private-version", "192.168.222.10/24", "9.9.9.9", "private-file.yaml", "\"interfaces\"", "\"dhcp4\"", "\"pending\""} {
		if strings.Contains(text, hidden) {
			t.Fatalf("route view leaked unrelated data %q: %s", hidden, text)
		}
	}
	if len(api.requests) != requests {
		t.Fatal("show contacted manager")
	}
	editorCommands(t, s, "edit office", "set metric 25")
	out.Reset()
	editorCommands(t, s, "show")
	if strings.Contains(out.String(), "default-uplink") || !strings.Contains(out.String(), "25") {
		t.Fatalf("selected route was not scoped or staged: %s", out.String())
	}
	if *api.view.Routes["office"].Metric != 100 || len(api.requests) != requests {
		t.Fatal("show saved staged changes")
	}
	editorCommands(t, s, "next")
	out.Reset()
	editorCommands(t, s, "show")
	if !strings.Contains(out.String(), "default-uplink") {
		t.Fatal("next did not restore full route table")
	}
}

func TestRouteShowExplainsEmptyAndIncompleteRoutes(t *testing.T) {
	s, _, out := editorFixture(t)
	api := &networkFixtureAPI{view: networkmanager.View{Version: "base", Interfaces: map[string]networkmanager.Interface{"ip1": {Name: "ip1", ID: "ip1"}}, Routes: map[string]networkmanager.Route{}}}
	s.SetNetworkClient(api)
	editorCommands(t, s, "config network route")
	out.Reset()
	editorCommands(t, s, "show", "edit ?")
	if !strings.Contains(out.String(), "No entries in the Netplan routes list.") || !strings.Contains(out.String(), "Use edit NAME") || !strings.Contains(out.String(), "Choose a new route name") {
		t.Fatalf("empty route guidance missing: %s", out.String())
	}
	editorCommands(t, s, "edit new-route")
	out.Reset()
	editorCommands(t, s, "show")
	if !strings.Contains(out.String(), "has no fields yet") {
		t.Fatalf("new route guidance missing: %s", out.String())
	}
	editorCommands(t, s, "set destination 10.60.0.0/16")
	out.Reset()
	editorCommands(t, s, "show")
	if !strings.Contains(out.String(), "Unassigned routes") || !strings.Contains(out.String(), "new-route") {
		t.Fatal(out.String())
	}
	editorCommands(t, s, "set interface ip1")
	out.Reset()
	editorCommands(t, s, "show")
	if !strings.Contains(out.String(), "Interface ip1") || !strings.Contains(out.String(), "10.60.0.0/16") {
		t.Fatal(out.String())
	}
	if len(api.requests) != 1 {
		t.Fatal("route show sent mutation requests")
	}
}
