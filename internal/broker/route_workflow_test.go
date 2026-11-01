package broker

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/networkmanager"
)

func TestUnnamedRouteUsesIDFromCreationThroughSaveAndDelete(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	api.view.Routes["default-uplink"] = networkmanager.Route{Name: "default-uplink", Interface: "ip1", Destination: "0.0.0.0/0", Gateway: "192.0.2.1"}
	editorCommands(t, s, "config network route", "add")
	name := s.networkEdit.item
	if !networkmanager.ValidItemName(name) || s.prompt() != "safeterm::alice(network/route/2)> " {
		t.Fatal("new route has no automatic ID", s.prompt())
	}
	out.Reset()
	editorCommands(t, s, "show", "?")
	if !strings.Contains(out.String(), "Route: 2 (new)") || strings.Contains(out.String(), name) || strings.Contains(out.String(), "add office") {
		t.Fatal("new route exposed internal names", out)
	}
	if _, err := s.executeLine(context.Background(), "next", nil); err == nil || s.networkEdit.item != name {
		t.Fatal("an empty route was silently lost", err)
	}
	editorCommands(t, s, "set destination 192.168.20.0/24", "next")
	requests := len(api.requests)
	if _, err := s.executeLine(context.Background(), "save", nil); err == nil || !strings.Contains(err.Error(), "route 2:") || len(api.requests) != requests {
		t.Fatal("incomplete route saved or exposed its internal name", err)
	}
	editorCommands(t, s, "edit 2", "set interface ip1", "set gateway 192.0.2.1", "set metric 100", "next", "edit 2", "show", "save")
	route := api.view.Routes[name]
	if route.Interface != "ip1" || route.Destination != "192.168.20.0/24" || route.Gateway != "192.0.2.1" || route.Metric == nil || *route.Metric != 100 {
		t.Fatal("automatic ID lost route fields", route)
	}
	editorCommands(t, s, "config network route")
	var id string
	for _, choice := range s.networkEdit.itemChoices() {
		if resolved, _ := s.networkEdit.resolveItem(choice); resolved == name {
			id = choice
		}
	}
	editorCommands(t, s, "edit "+id, "delete "+id)
	if s.networkEdit.item != "" || len(s.networkEdit.view.Routes) != 1 {
		t.Fatal("deleted selected route retained an active editor")
	}
	editorCommands(t, s, "save")
	if len(api.view.Routes) != 1 || api.view.Routes["default-uplink"].Destination != "0.0.0.0/0" {
		t.Fatal("deletion affected another route", api.view.Routes)
	}
}

func TestRouteCommandsAcceptOnlyDisplayIDs(t *testing.T) {
	s, api, _ := uxNetworkFixture(t)
	api.view.Routes["2"] = networkmanager.Route{Name: "2", Interface: "ip1", Destination: "10.0.0.0/8"}
	api.view.Routes["office"] = networkmanager.Route{Name: "office", Interface: "ip1", Destination: "192.168.20.0/24"}
	editorCommands(t, s, "config network route", "edit 2")
	if s.networkEdit.item != "office" {
		t.Fatal("numeric stored name overrode display ID")
	}
	for _, command := range []string{"edit office", "delete office", "add office", "add 3"} {
		before := s.networkEdit.item
		if _, err := s.executeLine(context.Background(), command, nil); err == nil || s.networkEdit.item != before || len(s.networkEdit.changes) != 0 {
			t.Fatal("named route command accepted or mutated state", command, err)
		}
	}
	editorCommands(t, s, "next", "delete 1", "add", "set destination 198.51.100.0/24", "set interface ip1", "next")
	if got := s.networkEdit.itemChoices(); !reflect.DeepEqual(got, []string{"2", "3"}) {
		t.Fatal("deleted IDs were reused", got)
	}
}

func TestDefaultRouteDeletionProtectionIsVisibleAndCannotBeBypassedLocally(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	api.view.Routes["default-v4"] = networkmanager.Route{Name: "default-v4", Interface: "ip1", Destination: "0.0.0.0/0", Gateway: "192.0.2.1"}
	api.view.Routes["default-v6"] = networkmanager.Route{Name: "default-v6", Interface: "ip1", Destination: "::/0", Gateway: "2001:db8::1"}
	api.view.Routes["office"] = networkmanager.Route{Name: "office", Interface: "ip1", Destination: "10.0.0.0/8"}
	editorCommands(t, s, "config network route", "show")
	if !strings.Contains(out.String(), "DELETION") || strings.Count(out.String(), "Protected (default)") != 2 {
		t.Fatal("default protection missing from table", out)
	}
	if got := s.completionCandidates(context.Background(), "delete ", ""); !reflect.DeepEqual(got, []string{"3"}) {
		t.Fatal("delete offered protected routes", got)
	}
	out.Reset()
	editorCommands(t, s, "delete ?")
	if strings.Contains(out.String(), "0.0.0.0/0") || strings.Contains(out.String(), "::/0") || !strings.Contains(out.String(), "10.0.0.0/8") {
		t.Fatal("delete help offered protected routes", out)
	}
	for _, id := range []string{"1", "2"} {
		if _, err := s.executeLine(context.Background(), "delete "+id, nil); err == nil || !strings.Contains(err.Error(), "default routes cannot be deleted") || s.editorDirty() {
			t.Fatal("default deletion changed state", id, err)
		}
	}
	editorCommands(t, s, "edit 1", "set destination 192.168.0.0/16")
	before := len(s.networkEdit.changes)
	if _, err := s.executeLine(context.Background(), "delete 1", nil); err == nil || len(s.networkEdit.changes) != before {
		t.Fatal("changing destination bypassed deletion protection", err)
	}
	editorCommands(t, s, "exit", "delete 3")
	if len(api.requests) != 1 || len(s.networkEdit.view.Routes) != 2 {
		t.Fatal("local deletion saved or affected protected routes")
	}
}
