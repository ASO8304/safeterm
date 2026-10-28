package broker

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/networkmanager"
)

func TestNetworkBackDiscardsOnlySelectedItemAndShowsParentChoices(t *testing.T) {
	s, _, out := editorFixture(t)
	api := &networkFixtureAPI{view: networkmanager.View{Version: "base", Interfaces: map[string]networkmanager.Interface{
		"ip0": {Name: "ip0", Addresses: []string{"192.168.222.10/24"}}, "ip1": {Name: "ip1"},
	}, Routes: map[string]networkmanager.Route{}}}
	s.SetNetworkClient(api)
	editorCommands(t, s, "config", "network", "interface", "edit ip1", "set mtu 1400", "next", "edit ip0", "append addresses 192.168.222.100/24")
	before := len(api.requests)
	out.Reset()
	editorCommands(t, s, "back ?")
	if s.networkEdit.item != "ip0" || len(s.networkEdit.changes) != 2 {
		t.Fatal("back help changed editor")
	}
	editorCommands(t, s, "back")
	e := s.networkEdit
	if s.prompt() != "safeterm::alice(network/interface)*> " || e.item != "" || len(e.changes) != 1 || e.view.Interfaces["ip1"].MTU == nil || len(e.view.Interfaces["ip0"].Addresses) != 1 {
		t.Fatalf("back discarded the wrong edits: %+v", e)
	}
	for _, want := range []string{"ip0", "ip1", "edit", "back", "Discarded unsaved edits for 'ip0'."} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("parent choice missing %q: %s", want, out)
		}
	}
	if len(api.requests) != before {
		t.Fatal("back contacted the privileged manager")
	}
	editorCommands(t, s, "edit ip0")
	if len(s.networkEdit.view.Interfaces["ip0"].Addresses) != 1 {
		t.Fatal("discarded item edits returned")
	}
	editorCommands(t, s, "back", "back")
	if s.networkEdit != nil || !reflect.DeepEqual(s.configMenu, []string{"network"}) {
		t.Fatal("did not return to network menu")
	}
	editorCommands(t, s, "back")
	if s.configMenu == nil || len(s.configMenu) != 0 {
		t.Fatal("did not return to config menu")
	}
	editorCommands(t, s, "back")
	if s.configMenu != nil || s.prompt() != "safeterm::alice> " || len(api.requests) != before {
		t.Fatal("navigation saved data or failed to reach root")
	}
}

func TestRouteBackRemovesUnsavedNewRoute(t *testing.T) {
	s, _, _ := editorFixture(t)
	api := &networkFixtureAPI{view: networkmanager.View{Version: "base", Interfaces: map[string]networkmanager.Interface{"ip0": {Name: "ip0"}}, Routes: map[string]networkmanager.Route{}}}
	s.SetNetworkClient(api)
	editorCommands(t, s, "config network route", "edit office", "set interface ip0", "set destination 10.0.0.0/8", "back")
	if len(s.networkEdit.view.Routes) != 0 || len(s.networkEdit.changes) != 0 || len(api.requests) != 1 {
		t.Fatal("back retained/saved a new route")
	}
}

func TestSectionBackDiscardsCurrentSectionAndPreservesOtherDrafts(t *testing.T) {
	s, path, out := editorFixture(t)
	before, _ := os.ReadFile(path)
	editorCommands(t, s, "config options", "edit Logging", "set Level debug", "next", "edit Server", "set Port 9090", "back")
	if s.editing.inSection || len(s.editing.changes) != 1 || s.editing.changes[0].Section != "Logging" {
		t.Fatal("back discarded the wrong section")
	}
	if !strings.Contains(out.String(), "Registered section") {
		t.Fatal("back did not show available sections")
	}
	current, _ := os.ReadFile(path)
	if string(current) != string(before) {
		t.Fatal("back wrote to disk")
	}
	editorCommands(t, s, "save")
	current, _ = os.ReadFile(path)
	if string(current) != strings.Replace(string(before), "Level = info", "Level = debug", 1) {
		t.Fatal("discarded section was saved or other draft was lost")
	}
}

func TestLogicalBackDiscardsContextAndReturnsToParentMenu(t *testing.T) {
	s, path, out := logicalConfigFixture(t)
	before, _ := os.ReadFile(path)
	editorCommands(t, s, "config application cat", "set enabled false", "back")
	if s.editing != nil || !reflect.DeepEqual(s.configMenu, []string{"application"}) || !strings.Contains(out.String(), "cat") {
		t.Fatal("wrong logical parent menu")
	}
	current, _ := os.ReadFile(path)
	if string(current) != string(before) {
		t.Fatal("back saved logical context")
	}
	editorCommands(t, s, "cat")
	if s.editing == nil || len(s.editing.changes) != 0 {
		t.Fatal("discarded logical edits returned")
	}
	if _, err := s.executeLine(context.Background(), "back extra", nil); err == nil || s.editing == nil {
		t.Fatal("invalid back mutated editor")
	}
}

func TestConfigurationMenuOffersLocalCompletionAndReadOnlyHelp(t *testing.T) {
	s, _, _ := editorFixture(t)
	s.SetNetworkClient(&networkFixtureAPI{})
	editorCommands(t, s, "config")
	for _, tc := range []struct{ line, prefix, want string }{{"n", "n", "network"}, {"edit ", "", "network,options"}, {"b", "b", "back"}} {
		if got := strings.Join(s.completionCandidates(context.Background(), tc.line, tc.prefix), ","); got != tc.want {
			t.Fatalf("completion %q=%s", tc.line, got)
		}
	}
	editorCommands(t, s, "network")
	for _, line := range []string{"?", "edit ?", "interface ?", "back ?"} {
		editorCommands(t, s, line)
	}
	if !reflect.DeepEqual(s.configMenu, []string{"network"}) || s.networkEdit != nil {
		t.Fatal("help entered another context")
	}
	if got := strings.Join(s.completionCandidates(context.Background(), "edit ", ""), ","); got != "interface,route" {
		t.Fatal(got)
	}
}
