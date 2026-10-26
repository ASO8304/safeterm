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
	editorCommands(t, s, "next", "end")
	if api.requests[len(api.requests)-1].Action != "save" || len(api.view.Interfaces["ens19"].Addresses) != 2 {
		t.Fatal(api.requests)
	}
	editorCommands(t, s, "config network route", "edit office", "set interface ens19", "set destination 198.51.100.0/24", "set gateway 192.0.2.254", "set metric 100", "end")
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
	editorCommands(t, s, "config network interface", "edit ens19", "set mtu 1450", "abort")
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
