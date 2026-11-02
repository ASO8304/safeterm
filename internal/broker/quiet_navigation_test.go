package broker

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/networkmanager"
)

func TestContextNavigationIsQuietAndExitClosesOnlyAtRoot(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	for _, step := range []struct{ command, prompt string }{
		{"config", "safeterm::alice(config)> "},
		{"edit network interface", "safeterm::alice(network/interface)> "},
		{"edit ip1", "safeterm::alice(network/interface/ip1)> "},
		{"exit", "safeterm::alice(network/interface)> "},
		{"exit", "safeterm::alice(network)> "},
		{"exit", "safeterm::alice(config)> "},
		{"exit", "safeterm::alice> "},
	} {
		out.Reset()
		editorCommands(t, s, step.command)
		if out.Len() != 0 || s.prompt() != step.prompt {
			t.Fatalf("%s: prompt=%q output=%q", step.command, s.prompt(), out.String())
		}
	}
	if len(api.requests) != 1 {
		t.Fatal("navigation called backend", api.requests)
	}
	exit, err := s.executeLine(context.Background(), "exit", nil)
	if !exit || err != nil {
		t.Fatal("root exit did not close session", exit, err)
	}

	s, _, out = logicalConfigFixture(t)
	for _, command := range []string{"config application cat", "exit", "exit", "exit"} {
		out.Reset()
		editorCommands(t, s, command)
		if out.Len() != 0 {
			t.Fatal("logical navigation printed a menu", out)
		}
	}
}

func TestTabOffersDomainItemsInsteadOfControlCommands(t *testing.T) {
	s, _, _ := uxNetworkFixture(t)
	check := func(want []string) {
		t.Helper()
		got := s.completionCandidates(context.Background(), "", "")
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("at %s: got %v, want %v", s.prompt(), got, want)
		}
	}
	editorCommands(t, s, "config")
	check([]string{"network", "options"})
	editorCommands(t, s, "network")
	check([]string{"interface", "route"})
	editorCommands(t, s, "interface")
	check([]string{"ip0", "ip1"})
	editorCommands(t, s, "ip1")
	check(nil)
	if got := s.completionCandidates(context.Background(), "set ", ""); len(got) != 6 {
		t.Fatal("set fields missing", got)
	}
	editorCommands(t, s, "cancel", "config options")
	check([]string{"Logging", "Server"})
	editorCommands(t, s, "Server")
	check(nil)
}

func TestRootMenuPrintsOnceOnLoginAndHelpRemainsExplicit(t *testing.T) {
	s, _, out := uxNetworkFixture(t)
	s.input = strings.NewReader("config network\rinterface\redit ip1\rexit\rexit\rexit\rexit\rwhoami\rexit\r")
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "Show available commands;") != 1 || strings.Contains(out.String(), "Select an existing network interface") || !strings.Contains(out.String(), "user: alice") || !strings.Contains(out.String(), "bye") {
		t.Fatal(out.String())
	}

	s, api, out := uxNetworkFixture(t)
	editorCommands(t, s, "config network interface", "edit ip1", "set mtu 1400")
	before := s.prompt()
	requests := len(api.requests)
	out.Reset()
	editorCommands(t, s, "?", "exit ?")
	if !strings.Contains(out.String(), exitDescription) || strings.Contains(out.String(), "whoami") || strings.Contains(out.String(), "back") || s.prompt() != before || len(api.requests) != requests {
		t.Fatal("help changed state or advertised removed commands", out)
	}
	for _, command := range []string{"whoami", "whoami ?", "back"} {
		if _, err := s.executeLine(context.Background(), command, nil); err == nil {
			t.Fatal("removed command accepted", command)
		}
	}
}

func TestNumberedRouteSelectionKeepsBackendIdentityAndClearDetails(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	name := "route-efce11ce5357"
	api.view.Routes[name] = networkmanager.Route{Name: name, Interface: "ip1", Destination: "0.0.0.0/0", Gateway: "192.0.2.1"}
	editorCommands(t, s, "config network route")
	if got := s.completionCandidates(context.Background(), "", ""); !reflect.DeepEqual(got, []string{"1"}) {
		t.Fatal(got)
	}
	out.Reset()
	editorCommands(t, s, "show")
	if strings.Contains(out.String(), name) || !strings.Contains(out.String(), "ID") || !strings.Contains(out.String(), "192.0.2.1") {
		t.Fatal(out.String())
	}
	editorCommands(t, s, "1")
	if s.networkEdit.item != name || s.prompt() != "safeterm::alice(network/route/1)> " {
		t.Fatal(s.prompt())
	}
	out.Reset()
	editorCommands(t, s, "show")
	for _, want := range []string{"Route: 1 (Default IPv4)", "Destination:", "Gateway:", "Interface:", "Metric:", "0.0.0.0/0", "192.0.2.1", "ip1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	if strings.Contains(out.String(), name) {
		t.Fatal("generated route ID leaked into detail")
	}
	editorCommands(t, s, "set gateway 192.0.2.254", "next", "save")
	last := api.requests[len(api.requests)-1]
	if len(api.requests) != 2 || last.Action != "save" || len(last.Changes) != 1 || last.Changes[0].Name != name || api.view.Routes[name].Gateway != "192.0.2.254" {
		t.Fatal("selector changed privileged route identity", api.requests)
	}
}

func TestRouteCreationIsExplicitAndSelectorsDoNotShiftAfterDelete(t *testing.T) {
	s, api, _ := uxNetworkFixture(t)
	for _, name := range []string{"route-aaaaaaaaaaaa", "route-bbbbbbbbbbbb"} {
		api.view.Routes[name] = networkmanager.Route{Name: name, Interface: "ip1", Destination: "10.0.0.0/8"}
	}
	editorCommands(t, s, "config network route")
	for _, command := range []string{"edit missing", "add ../unsafe", "add 1"} {
		if _, err := s.executeLine(context.Background(), command, nil); err == nil || s.networkEdit.item != "" {
			t.Fatal("invalid selection/creation changed context", command, err)
		}
	}
	editorCommands(t, s, "delete 1", "add", "set interface ip1", "set destination 198.51.100.0/24", "next")
	if got := s.networkEdit.itemChoices(); !reflect.DeepEqual(got, []string{"2", "3"}) {
		t.Fatal("route selectors shifted", got)
	}
	editorCommands(t, s, "edit 2")
	if s.networkEdit.item != "route-bbbbbbbbbbbb" {
		t.Fatal("ID 2 selected another route")
	}
	if len(api.requests) != 1 {
		t.Fatal("route editing mutated backend")
	}
}

func TestNumberedRouteStillEnforcesReadOnlyRestriction(t *testing.T) {
	s, api, _ := uxNetworkFixture(t)
	name := "route-aaaaaaaaaaaa"
	api.view.Routes[name] = networkmanager.Route{Name: name, Interface: "ip0", Destination: "0.0.0.0/0", ReadOnly: "policy restricted"}
	editorCommands(t, s, "config network route", "edit 1")
	before := s.networkEdit.view
	if _, err := s.executeLine(context.Background(), "set metric 50", nil); err == nil || !reflect.DeepEqual(before, s.networkEdit.view) || s.editorDirty() || len(api.requests) != 1 {
		t.Fatal("selector bypassed read-only policy", err)
	}
}

func TestNewRouteWithoutFieldsCannotBeSilentlySaved(t *testing.T) {
	s, api, _ := uxNetworkFixture(t)
	editorCommands(t, s, "config network route", "add")
	if _, err := s.executeLine(context.Background(), "save", nil); err == nil || s.networkEdit == nil || len(api.requests) != 1 {
		t.Fatal("empty route creation pretended to save", err)
	}
}

func TestSectionNameDoesNotOverrideShowCommand(t *testing.T) {
	s, _, out := editorFixture(t)
	editorCommands(t, s, "config options")
	s.editing.description.Fields[0].Section = "show"
	out.Reset()
	editorCommands(t, s, "show")
	if s.editing.inSection || !strings.Contains(out.String(), "8080") {
		t.Fatal("bare section selection shadowed a command", out)
	}
}
